package controllers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

func TestPrepareLBBodySortsEndpoints(t *testing.T) {
	ctx := context.Background()
	scheme := newTestScheme(t)

	ready := true
	objs := []client.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer},
		},
	}
	var endpoints []discoveryv1.Endpoint
	for i, ip := range []string{"10.0.0.13", "10.0.0.11", "10.0.0.12"} {
		name := fmt.Sprintf("node-%d", i)
		objs = append(objs, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: ip}}},
		})
		endpoints = append(endpoints, discoveryv1.Endpoint{
			Addresses:  []string{fmt.Sprintf("172.16.0.%d", i)},
			NodeName:   &name,
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
		})
	}
	objs = append(objs, &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc",
			Namespace: "default",
			Labels:    map[string]string{"kubernetes.io/service-name": "web"},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints:   endpoints,
	})

	r := &ServiceReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()}

	body, err := r.prepareLBBody(ctx, objs[0].(*corev1.Service), "198.18.1.2", "spx-subnet-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.11", "10.0.0.12", "10.0.0.13"}, body.Spec.Endpoints)
}

func TestLBUpToDate(t *testing.T) {
	desired := api.CreateLoadBalancerBody{}
	desired.General.ProductName = "k8s-lb-default-web"
	desired.Spec = api.LoadBalancerSpec{
		VIP:       "198.18.1.2",
		SubnetEID: "spx-subnet-1",
		Endpoints: []string{"10.0.0.11", "10.0.0.12"},
		Ports: []api.RulePort{
			{Port: 80, Protocol: "TCP", TargetPort: 30080},
			{Port: 443, Protocol: "TCP", TargetPort: 30443},
		},
	}

	matching := func() *api.LBFullResponse {
		current := &api.LBFullResponse{ProductName: "k8s-lb-default-web"}
		current.LoadBalancer.SubnetEID = "spx-subnet-1"
		current.LoadBalancer.Spec = api.LoadBalancerSpec{
			VIP:       "198.18.1.2",
			Endpoints: []string{"10.0.0.12", "10.0.0.11"},
			Ports: []api.RulePort{
				{Port: 443, Protocol: "tcp", TargetPort: 30443},
				{Port: 80, Protocol: "TCP", TargetPort: 30080},
			},
		}
		return current
	}

	tests := []struct {
		name   string
		mutate func(*api.LBFullResponse)
		want   bool
	}{
		{name: "same state in another order", mutate: func(*api.LBFullResponse) {}, want: true},
		{name: "subnet not reported by the API", mutate: func(c *api.LBFullResponse) { c.LoadBalancer.SubnetEID = "" }, want: true},
		{name: "subnet reported in spec", mutate: func(c *api.LBFullResponse) {
			c.LoadBalancer.SubnetEID = ""
			c.LoadBalancer.Spec.SubnetEID = "spx-subnet-1"
		}, want: true},
		{name: "different product name", mutate: func(c *api.LBFullResponse) { c.ProductName = "other" }, want: false},
		{name: "different VIP", mutate: func(c *api.LBFullResponse) { c.LoadBalancer.Spec.VIP = "198.18.1.3" }, want: false},
		{name: "different subnet", mutate: func(c *api.LBFullResponse) { c.LoadBalancer.SubnetEID = "spx-subnet-2" }, want: false},
		{name: "missing endpoint", mutate: func(c *api.LBFullResponse) { c.LoadBalancer.Spec.Endpoints = []string{"10.0.0.11"} }, want: false},
		{name: "different endpoint", mutate: func(c *api.LBFullResponse) {
			c.LoadBalancer.Spec.Endpoints = []string{"10.0.0.11", "10.0.0.13"}
		}, want: false},
		{name: "different node port", mutate: func(c *api.LBFullResponse) { c.LoadBalancer.Spec.Ports[1].TargetPort = 30081 }, want: false},
		{name: "missing port", mutate: func(c *api.LBFullResponse) { c.LoadBalancer.Spec.Ports = c.LoadBalancer.Spec.Ports[:1] }, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current := matching()
			tt.mutate(current)
			assert.Equal(t, tt.want, lbUpToDate(desired, current))
		})
	}
}

// fakeLBAPI serves GET and POST /load-balancer/{eid} and counts the updates.
type fakeLBAPI struct {
	mu      sync.Mutex
	current string
	updates int
}

func (f *fakeLBAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != fmt.Sprintf("/org-1/api/spx-ctrl/az-1/%s/load-balancer/spx-lb-1", testProjectID) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(f.current))
	case http.MethodPost:
		f.updates++
		w.WriteHeader(http.StatusOK)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// reconcileFixtures returns an existing internal LoadBalancer service (already
// created in the external API as spx-lb-1) with one ready endpoint on node-1.
func reconcileFixtures(status corev1.ServiceStatus) []client.Object {
	nodeName := "node-1"
	ready := true
	return []client.Object{
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: nodeName},
			Status:     corev1.NodeStatus{Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: "10.0.0.11"}}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "web",
				Namespace:  "default",
				Finalizers: []string{lbFinalizer},
				Annotations: map[string]string{
					lbEIDAnnotation:     "spx-lb-1",
					lbVIPAnnotation:     "198.18.1.2",
					subnetEIDAnnotation: "spx-subnet-1",
				},
			},
			Spec: corev1.ServiceSpec{
				Type:  corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{{Port: 80, NodePort: 30080, Protocol: corev1.ProtocolTCP}},
			},
			Status: status,
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-abc",
				Namespace: "default",
				Labels:    map[string]string{"kubernetes.io/service-name": "web"},
			},
			AddressType: discoveryv1.AddressTypeIPv4,
			Endpoints: []discoveryv1.Endpoint{{
				Addresses:  []string{"172.16.0.5"},
				NodeName:   &nodeName,
				Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			}},
		},
	}
}

// fakeLBJSON is the GET /load-balancer/spx-lb-1 response matching reconcileFixtures,
// except for the given endpoint.
func fakeLBJSON(endpoint string) string {
	return fmt.Sprintf(`{"eid": "spx-lb-1", "productName": "k8s-lb-default-web", "loadBalancer": {"subnetEId": "spx-subnet-1",
		"spec": {"vip": "198.18.1.2", "endpoints": [%q], "ports": [{"name": "80-30080-tcp", "port": 80, "protocol": "TCP", "targetPort": 30080}]}}}`, endpoint)
}

// newReconcileTest builds a reconciler on the fixtures and a fake API, and counts
// the status updates sent to Kubernetes.
func newReconcileTest(t *testing.T, lbAPI *fakeLBAPI, status corev1.ServiceStatus) (*ServiceReconciler, client.Client, *int) {
	scheme := newTestScheme(t)
	server := httptest.NewServer(lbAPI)
	t.Cleanup(server.Close)

	statusUpdates := 0
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(reconcileFixtures(status)...).
		WithStatusSubresource(&corev1.Service{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, c client.Client, subResource string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				statusUpdates++
				return c.SubResource(subResource).Update(ctx, obj, opts...)
			},
		}).Build()
	r := &ServiceReconciler{
		Client:    c,
		Scheme:    scheme,
		APIClient: api.NewClient(server.URL, "token", "org-1", "az-1", testProjectID, false),
	}
	return r, c, &statusUpdates
}

func TestReconcileUpdatesLBOnlyWhenNeeded(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		endpoint    string
		wantUpdates int
	}{
		{name: "up to date", endpoint: "10.0.0.11", wantUpdates: 0},
		{name: "endpoints changed", endpoint: "10.0.0.99", wantUpdates: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbAPI := &fakeLBAPI{current: fakeLBJSON(tt.endpoint)}
			r, c, _ := newReconcileTest(t, lbAPI, corev1.ServiceStatus{})

			key := types.NamespacedName{Namespace: "default", Name: "web"}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			require.NoError(t, err)
			assert.Equal(t, tt.wantUpdates, lbAPI.updates)

			var svc corev1.Service
			require.NoError(t, c.Get(ctx, key, &svc))
			require.Len(t, svc.Status.LoadBalancer.Ingress, 1)
			assert.Equal(t, "198.18.1.2", svc.Status.LoadBalancer.Ingress[0].IP)
		})
	}
}

func TestReconcileUpdatesStatusOnlyWhenNeeded(t *testing.T) {
	ctx := context.Background()
	key := types.NamespacedName{Namespace: "default", Name: "web"}
	vipMode := corev1.LoadBalancerIPModeVIP
	withIngress := func(ingress ...corev1.LoadBalancerIngress) corev1.ServiceStatus {
		return corev1.ServiceStatus{LoadBalancer: corev1.LoadBalancerStatus{Ingress: ingress}}
	}

	tests := []struct {
		name              string
		status            corev1.ServiceStatus
		wantStatusUpdates int
	}{
		{name: "no ingress yet", status: corev1.ServiceStatus{}, wantStatusUpdates: 1},
		{name: "same IP (ipMode defaulted by the API server)", status: withIngress(corev1.LoadBalancerIngress{IP: "198.18.1.2", IPMode: &vipMode}), wantStatusUpdates: 0},
		{name: "other IP", status: withIngress(corev1.LoadBalancerIngress{IP: "198.18.9.9"}), wantStatusUpdates: 1},
		{name: "extra ingress", status: withIngress(corev1.LoadBalancerIngress{IP: "198.18.1.2"}, corev1.LoadBalancerIngress{IP: "198.18.9.9"}), wantStatusUpdates: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, c, statusUpdates := newReconcileTest(t, &fakeLBAPI{current: fakeLBJSON("10.0.0.11")}, tt.status)

			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatusUpdates, *statusUpdates)

			var svc corev1.Service
			require.NoError(t, c.Get(ctx, key, &svc))
			require.Len(t, svc.Status.LoadBalancer.Ingress, 1)
			assert.Equal(t, "198.18.1.2", svc.Status.LoadBalancer.Ingress[0].IP)

			// A second reconciliation with nothing changed must not write the status again
			_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: key})
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatusUpdates, *statusUpdates)
		})
	}
}
