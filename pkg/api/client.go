package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	BaseURL    string
	Token      string
	OrgaID     string
	AZ         string
	ProjectID  string
	Debug      bool
	HTTPClient *http.Client
}

func NewClient(baseURL, token, orgaID, az, projectID string, debug bool) *Client {
	return &Client{
		BaseURL:    baseURL,
		Token:      token,
		OrgaID:     orgaID,
		AZ:         az,
		ProjectID:  projectID,
		Debug:      debug,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) GetClusterInfo(clusterID string) (*ClusterAppResponse, error) {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/kaas/%s/app", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID, clusterID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call GET %s\n", url)
	}

	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get cluster info, status: %d", resp.StatusCode)
	}

	var clusterResp ClusterAppResponse
	if err := json.NewDecoder(resp.Body).Decode(&clusterResp); err != nil {
		return nil, err
	}

	return &clusterResp, nil
}

func (c *Client) CreateLoadBalancer(body CreateLoadBalancerBody) (string, error) {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/load-balancer", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID)

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call POST %s\nBody: %s\n", url, string(data))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create load balancer, status: %d", resp.StatusCode)
	}

	var createResp CreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", err
	}

	return createResp.EID, nil
}

func (c *Client) CreateEIP(body CreateEIPBody) (string, error) {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/eip", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID)

	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call POST %s\nBody: %s\n", url, string(data))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to create eip, status: %d", resp.StatusCode)
	}

	var createResp CreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", err
	}

	return createResp.EID, nil
}

func (c *Client) UpdateLoadBalancer(eid string, body CreateLoadBalancerBody) error {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/load-balancer/%s", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID, eid)

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call POST %s\nBody: %s\n", url, string(data))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to update load balancer, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) DeleteEIP(eid string) error {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/eip/%s", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID, eid)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call DELETE %s\n", url)
	}

	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete eip, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetLoadBalancer(eid string) (*LBFullResponse, error) {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/load-balancer/%s", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID, eid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call GET %s\n", url)
	}

	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get load balancer, status: %d", resp.StatusCode)
	}

	var lbResp LBFullResponse
	if err := json.NewDecoder(resp.Body).Decode(&lbResp); err != nil {
		return nil, err
	}

	return &lbResp, nil
}

func (c *Client) DeleteLoadBalancer(eid string) error {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/load-balancer/%s", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID, eid)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call DELETE %s\n", url)
	}

	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete load balancer, status: %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) GetEIP(eid string) (*EIPFullResponse, error) {
	url := fmt.Sprintf("%s/%s/api/spx-ctrl/%s/%s/eip/%s", c.BaseURL, c.OrgaID, c.AZ, c.ProjectID, eid)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.Debug {
		fmt.Printf("DEBUG: API Call GET %s\n", url)
	}

	req.Header.Set("Authorization", "api-token "+c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var bodyBytes []byte
	if c.Debug {
		bodyBytes, _ = io.ReadAll(resp.Body)
		fmt.Printf("DEBUG: API Response Status: %d\nBody: %s\n", resp.StatusCode, string(bodyBytes))
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get eip, status: %d", resp.StatusCode)
	}

	var eipResp EIPFullResponse
	if err := json.NewDecoder(resp.Body).Decode(&eipResp); err != nil {
		return nil, err
	}

	return &eipResp, nil
}
