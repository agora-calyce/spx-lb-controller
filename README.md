# spx-lb-controller

Kubernetes LoadBalancer Controller.

## Description
This Kubernetes controller creates and configures load balancers through an external API as soon as a `LoadBalancer` type service is configured in the cluster.

## Features
- Watches Kubernetes services of type `LoadBalancer`.
- Calls an API to create and configure the load balancer.
- Updates the service status with the IP or hostname of the created load balancer.

## Prerequisites
- Kubernetes 1.33+
- Access to the load-balancing API.
- Cluster Nodes must carry the `cluster.x-k8s.io/cluster-name` annotation.

## Known issues
- None.

## Installation

The Helm chart is published on GHCR and attached to each [GitHub Release](https://github.com/agora-calyce/spx-lb-controller/releases).

```sh
helm install spx-lb-controller oci://ghcr.io/agora-calyce/charts/spx-lb-controller \
  --version <version> \
  --namespace spx-lb-controller --create-namespace \
  --set api.token=<token> \
  --set api.orgaId=<organization-id> \
  --set api.az=<az-code>
```

See the [chart README](chart/spx-lb-controller/README.md) for all values.

## Service configuration (Annotations)

The controller uses several annotations on `Service` resources of type `LoadBalancer` to configure the behavior of the external load balancer.

### User annotations

| Annotation | Possible values | Default | Description |
|------------|-----------------|---------|-------------|
| `superphenix.net/lb-expose` | `internal`, `public` | `internal` | Defines whether the service must be exposed publicly through an EIP (External IP) or remain internal. |
| `superphenix.net/lb-vip` | IP (CIDR `198.18.0.0/16`) | Auto-generated | Sets a fixed VIP address for the load balancer. If not set, the VIP of the reused load balancer is kept (see `lb-eid`), otherwise an IP is picked at random in the `198.18.0.0/16` block. |
| `superphenix.net/lb-eid` | EID of an existing SPX load balancer | Set by the controller | Reuses an existing load balancer instead of creating a new one (see [Reusing an existing load balancer](#reusing-an-existing-load-balancer)). |

### Status annotations (managed automatically)

These annotations are added by the controller and must not be modified manually except in exceptional cases.

| Annotation | Description |
|------------|-------------|
| `superphenix.net/lb-eid` | Unique identifier (EID) of the LoadBalancer created (or reused) in the external API. |
| `superphenix.net/subnet-eid` | Subnet identifier (`spx-` + UUID v5) computed from the cluster info and used to create the load balancer and the EIP. |
| `cluster.x-k8s.io/cluster-name` | Annotation present on Nodes, used to retrieve the cluster identifier. |
| `cluster.x-k8s.io/cluster-namespace` | Annotation present on Nodes, used to retrieve the ProjectID (after removing the `spx-` prefix) at startup. The controller exits with an error if no Node, no annotation or no valid UUID is found. |
| `superphenix.net/eip-eid` | Unique identifier (EID) of the associated EIP (only in `public` mode). |
| `superphenix.net/lb-vip` | Stores the automatically generated VIP so that it stays immutable. |

## Behavior and logic

### Subnet
The SPX API requires every load balancer to be attached to a subnet, and its endpoints (the Node internal IPs) must belong to that subnet.
The controller uses the first subnet of the first group of the cluster (`kaas/{cluster}/app`), computes its EID as `spx-` + `uuidv5(projectID, subnetLocalID)` and stores it in the `superphenix.net/subnet-eid` annotation.

### Public vs internal exposure
- **`internal` mode**: The controller creates an external load balancer. The external IP of the Kubernetes service (`.status.loadBalancer.ingress`) is the load balancer VIP.
- **`public` mode**:
    1. The controller creates the load balancer.
    2. It then creates an **EIP** (External IP) resource bound to the internal IP (VIP) of the load balancer.
    3. It waits for the EIP to be marked as `ready` (`eip.status.ready`).
    4. The external IP of the Kubernetes service is the public IP provided by the EIP (`eip.status.ip`, or `eip.spec.v4ip` as a fallback).

### VIP management
The VIP is the internal IP address of the load balancer. It must be within the `198.18.0.0/16` CIDR.
Once assigned (either through the manual annotation or automatic generation), it is stored in the service annotations and never changes during the lifecycle of the external resource.

### Updates
The controller reconciles a service on every change of the service or of its EndpointSlices. It only sends an update to the SPX API when the external load balancer differs from the desired state: product name, VIP, subnet, endpoints (Node internal IPs) or ports (port, protocol, NodePort), compared regardless of their order.
The service status (`.status.loadBalancer.ingress`) is only written when its IP changes.

### Reusing an existing load balancer
Setting `superphenix.net/lb-eid` on a new Service makes the controller adopt that load balancer instead of creating one:
- its VIP is kept (and stored in `superphenix.net/lb-vip`), unless `superphenix.net/lb-vip` is set;
- its product name, endpoints and ports are then updated to match the Service, like for any managed load balancer;
- it is **deleted with the Service**, as the controller then owns it.

In `public` mode, an EIP is created unless `superphenix.net/eip-eid` is also set.

### Cleanup
When the Kubernetes service is deleted, or if its type changes and it is no longer a `LoadBalancer`, the controller automatically deletes the associated resources (LoadBalancer and EIP) in the external API.
