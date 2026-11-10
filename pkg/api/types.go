package api

type RulePort struct {
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	TargetPort int    `json:"targetPort"`
}

type LoadBalancerSpec struct {
	Endpoints []string   `json:"endpoints,omitempty"`
	Ports     []RulePort `json:"ports,omitempty"`
	VIP       string     `json:"vip,omitempty"`
	// Subnet the load balancer attaches to, required by the API. Endpoints must be in it.
	SubnetEID string `json:"subnetEId"`
}

type CreateLoadBalancerBody struct {
	General struct {
		ProductName string `json:"productName"`
	} `json:"general"`
	Spec LoadBalancerSpec `json:"spec"`
}

type ClusterAppResponse struct {
	Spec struct {
		Groups []struct {
			Subnets []struct {
				ID string `json:"id"`
			} `json:"subnets"`
		} `json:"groups"`
	} `json:"spec"`
}

type CreateResponse struct {
	EID string `json:"eid"`
}

type CreateEIPBody struct {
	General struct {
		ProductName string `json:"productName"`
		SubnetEID   string `json:"subnetEId"`
	} `json:"general"`
	Spec struct {
		InternalIP string `json:"internalIP"`
	} `json:"spec"`
}

type LBFullResponse struct {
	EID          string `json:"eid"`
	ProductName  string `json:"productName"`
	LoadBalancer struct {
		SubnetEID string           `json:"subnetEId"`
		Spec      LoadBalancerSpec `json:"spec"`
	} `json:"loadBalancer"`
}

// EIPFullResponse is the response of GET /eip/{eid}. The EIP itself is a
// kube-ovn IptablesEIP nested under "eip".
type EIPFullResponse struct {
	EID         string `json:"eid"`
	ProductName string `json:"productName"`
	EIP         struct {
		Spec struct {
			V4IP string `json:"v4ip"`
		} `json:"spec"`
		Status struct {
			Ready bool   `json:"ready"`
			IP    string `json:"ip"`
		} `json:"status"`
	} `json:"eip"`
}

// ExternalIP returns the public IP of the EIP, from its status or, if not
// reported yet, from its spec.
func (e *EIPFullResponse) ExternalIP() string {
	if e.EIP.Status.IP != "" {
		return e.EIP.Status.IP
	}
	return e.EIP.Spec.V4IP
}
