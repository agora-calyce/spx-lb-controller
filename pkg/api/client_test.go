package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateLoadBalancerJSON(t *testing.T) {
	expectedJSON := `{
  "general": {
    "productName": "test-lb"
  },
  "spec": {
    "endpoints": ["1.2.3.4", "5.6.7.8"],
    "ports": [
      {
        "port": 80,
        "protocol": "TCP",
        "targetPort": 8080
      }
    ],
    "vip": "10.0.0.1",
    "subnetEId": "spx-subnet-123"
  }
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/org-1/api/spx-ctrl/az-1/proj-1/load-balancer", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "api-token test-token", r.Header.Get("Authorization"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		// Compare JSON objects to avoid formatting/whitespace issues
		var actual, expected interface{}
		err = json.Unmarshal(body, &actual)
		assert.NoError(t, err)
		err = json.Unmarshal([]byte(expectedJSON), &expected)
		assert.NoError(t, err)

		assert.Equal(t, expected, actual)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eid": "lb-123"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", "org-1", "az-1", "proj-1", false)
	body := CreateLoadBalancerBody{}
	body.General.ProductName = "test-lb"
	body.Spec = LoadBalancerSpec{
		Endpoints: []string{"1.2.3.4", "5.6.7.8"},
		Ports: []RulePort{
			{
				Port:       80,
				Protocol:   "TCP",
				TargetPort: 8080,
			},
		},
		VIP:       "10.0.0.1",
		SubnetEID: "spx-subnet-123",
	}

	eid, err := client.CreateLoadBalancer(body)
	assert.NoError(t, err)
	assert.Equal(t, "lb-123", eid)
}

func TestUpdateLoadBalancerJSON(t *testing.T) {
	expectedJSON := `{
  "general": {
    "productName": "updated-lb"
  },
  "spec": {
    "ports": [
      {
        "port": 443,
        "protocol": "TCP",
        "targetPort": 8443
      }
    ],
    "subnetEId": "spx-subnet-123"
  }
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/org-1/api/spx-ctrl/az-1/proj-1/load-balancer/lb-123", r.URL.Path)

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		var actual, expected interface{}
		err = json.Unmarshal(body, &actual)
		assert.NoError(t, err)
		err = json.Unmarshal([]byte(expectedJSON), &expected)
		assert.NoError(t, err)

		assert.Equal(t, expected, actual)

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", "org-1", "az-1", "proj-1", false)

	body := CreateLoadBalancerBody{}
	body.General.ProductName = "updated-lb"
	body.Spec = LoadBalancerSpec{
		Ports: []RulePort{
			{
				Port:       443,
				Protocol:   "TCP",
				TargetPort: 8443,
			},
		},
		SubnetEID: "spx-subnet-123",
	}

	err := client.UpdateLoadBalancer("lb-123", body)
	assert.NoError(t, err)
}

func TestCreateEIPJSON(t *testing.T) {
	expectedJSON := `{
  "general": {
    "productName": "test-eip",
    "subnetEId": "calculated-subnet-id"
  },
  "spec": {
    "internalIP": "198.18.0.1"
  }
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/org-1/api/spx-ctrl/az-1/proj-1/eip", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)

		var actual, expected interface{}
		err = json.Unmarshal(body, &actual)
		assert.NoError(t, err)
		err = json.Unmarshal([]byte(expectedJSON), &expected)
		assert.NoError(t, err)

		assert.Equal(t, expected, actual)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"eid": "eip-123"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", "org-1", "az-1", "proj-1", false)

	body := CreateEIPBody{}
	body.General.ProductName = "test-eip"
	body.General.SubnetEID = "calculated-subnet-id"
	body.Spec.InternalIP = "198.18.0.1"

	eid, err := client.CreateEIP(body)
	assert.NoError(t, err)
	assert.Equal(t, "eip-123", eid)
}

func TestGetEIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/org-1/api/spx-ctrl/az-1/proj-1/eip/eip-123", r.URL.Path)
		assert.Equal(t, "api-token test-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "local-123",
			"eid": "eip-123",
			"productName": "test-eip",
			"codeAZ": "az-1",
			"gitops": "false",
			"eip": {
				"metadata": {"name": "eip-123"},
				"spec": {
					"v4ip": "1.2.3.4",
					"externalSubnet": "external"
				},
				"status": {
					"ready": true,
					"ip": "1.2.3.4"
				}
			},
			"fip": {"spec": {"eip": "eip-123", "internalIp": "198.18.0.1"}}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", "org-1", "az-1", "proj-1", false)

	eip, err := client.GetEIP("eip-123")
	assert.NoError(t, err)
	assert.Equal(t, "eip-123", eip.EID)
	assert.Equal(t, "test-eip", eip.ProductName)
	assert.True(t, eip.EIP.Status.Ready)
	assert.Equal(t, "1.2.3.4", eip.ExternalIP())
}

func TestEIPExternalIP(t *testing.T) {
	t.Run("Prefer status IP", func(t *testing.T) {
		eip := EIPFullResponse{}
		eip.EIP.Spec.V4IP = "5.6.7.8"
		eip.EIP.Status.IP = "1.2.3.4"
		assert.Equal(t, "1.2.3.4", eip.ExternalIP())
	})

	t.Run("Fallback to spec v4ip", func(t *testing.T) {
		eip := EIPFullResponse{}
		eip.EIP.Spec.V4IP = "5.6.7.8"
		assert.Equal(t, "5.6.7.8", eip.ExternalIP())
	})

	t.Run("Empty when not assigned", func(t *testing.T) {
		eip := EIPFullResponse{}
		assert.Equal(t, "", eip.ExternalIP())
	})
}

func TestDeleteEIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "DELETE", r.Method)
		assert.Equal(t, "/org-1/api/spx-ctrl/az-1/proj-1/eip/eip-123", r.URL.Path)
		assert.Equal(t, "api-token test-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", "org-1", "az-1", "proj-1", false)

	err := client.DeleteEIP("eip-123")
	assert.NoError(t, err)
}

func TestGetLoadBalancer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "/org-1/api/spx-ctrl/az-1/proj-1/load-balancer/lb-123", r.URL.Path)
		assert.Equal(t, "api-token test-token", r.Header.Get("Authorization"))

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"eid": "lb-123",
			"productName": "test-lb",
			"loadBalancer": {
				"subnetEId": "subnet-123",
				"spec": {
					"vip": "198.18.0.1",
					"endpoints": ["1.1.1.1"],
					"ports": [{"port": 80, "protocol": "TCP", "targetPort": 8080}]
				}
			}
		}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", "org-1", "az-1", "proj-1", false)

	lb, err := client.GetLoadBalancer("lb-123")
	assert.NoError(t, err)
	assert.Equal(t, "lb-123", lb.EID)
	assert.Equal(t, "198.18.0.1", lb.LoadBalancer.Spec.VIP)
	assert.Equal(t, "subnet-123", lb.LoadBalancer.SubnetEID)
}
