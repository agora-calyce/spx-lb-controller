package controllers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"spx-lb-controller/pkg/api"
)

const testProjectID = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"

func newTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	return scheme
}

func TestGetOrGenerateVIP(t *testing.T) {
	r := &ServiceReconciler{}
	ctx := context.Background()

	t.Run("Use existing annotation", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					lbVIPAnnotation: "198.18.10.10",
				},
			},
		}
		vip, err := r.getOrGenerateVIP(ctx, svc)
		assert.NoError(t, err)
		assert.Equal(t, "198.18.10.10", vip)
	})

	t.Run("Generate new VIP in CIDR", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}
		vip, err := r.getOrGenerateVIP(ctx, svc)
		assert.NoError(t, err)

		ip := net.ParseIP(vip)
		assert.NotNil(t, ip)

		_, ipnet, _ := net.ParseCIDR(vipCIDR)
		assert.True(t, ipnet.Contains(ip), "Generated VIP %s should be in %s", vip, vipCIDR)
	})
}

func TestGetOrComputeSubnetID(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)

	t.Run("Use existing annotation", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc",
				Namespace: "default",
				Annotations: map[string]string{
					subnetEIDAnnotation: "spx-existing",
				},
			},
		}
		// No API client: it must not be called
		r := &ServiceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

		subnetID, err := r.getOrComputeSubnetID(ctx, svc)
		assert.NoError(t, err)
		assert.Equal(t, "spx-existing", subnetID)
	})

	t.Run("Compute from cluster info and store annotation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, fmt.Sprintf("/org-1/api/spx-ctrl/az-1/%s/kaas/cluster-1/app", testProjectID), r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"spec": {"groups": [{"subnets": [{"order": 0, "id": "subnet-local-1"}]}]}}`))
		}))
		defer server.Close()

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "node-1",
				Annotations: map[string]string{clusterNameAnno: "cluster-1"},
			},
		}
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, svc).Build()
		r := &ServiceReconciler{
			Client:    c,
			APIClient: api.NewClient(server.URL, "token", "org-1", "az-1", testProjectID, false),
		}

		subnetID, err := r.getOrComputeSubnetID(ctx, svc)
		require.NoError(t, err)

		expected := "spx-" + uuid.NewSHA1(uuid.MustParse(testProjectID), []byte("subnet-local-1")).String()
		assert.Equal(t, expected, subnetID)

		var stored corev1.Service
		require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(svc), &stored))
		assert.Equal(t, expected, stored.Annotations[subnetEIDAnnotation])
	})

	t.Run("Error when cluster has no subnet", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"spec": {"groups": []}}`))
		}))
		defer server.Close()

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "node-1",
				Annotations: map[string]string{clusterNameAnno: "cluster-1"},
			},
		}
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		}
		r := &ServiceReconciler{
			Client:    fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, svc).Build(),
			APIClient: api.NewClient(server.URL, "token", "org-1", "az-1", testProjectID, false),
		}

		_, err := r.getOrComputeSubnetID(ctx, svc)
		assert.Error(t, err)
	})
}

func TestPrepareLBBody(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)

	nodeName := "node-1"
	ready := true
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeHostName, Address: "node-1"},
				{Type: corev1.NodeInternalIP, Address: "10.0.0.11"},
			},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	eps := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc",
			Namespace: "default",
			Labels:    map[string]string{"kubernetes.io/service-name": "web"},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses:  []string{"172.16.0.5"},
				NodeName:   &nodeName,
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			},
		},
	}

	r := &ServiceReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, svc, eps).Build(),
	}

	body, err := r.prepareLBBody(ctx, svc, "198.18.1.2", "spx-subnet-1")
	require.NoError(t, err)

	assert.Equal(t, "k8s-lb-default-web", body.General.ProductName)
	assert.Equal(t, "198.18.1.2", body.Spec.VIP)
	assert.Equal(t, "spx-subnet-1", body.Spec.SubnetEID)
	assert.Equal(t, []string{"10.0.0.11"}, body.Spec.Endpoints)
	assert.Equal(t, []api.RulePort{{Port: 80, Protocol: "TCP", TargetPort: 30080}}, body.Spec.Ports)
}
