# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
- Fix an update loop when a Service references an existing load balancer through `superphenix.net/lb-eid` without `superphenix.net/lb-vip`: a new random VIP was generated on every reconciliation, overwriting the VIP of the load balancer and updating it continuously. The VIP of the existing load balancer is now reused and stored in `superphenix.net/lb-vip`.
- Document the reuse of an existing load balancer through the `superphenix.net/lb-eid` annotation.

## [v0.1.2]
- Only write the service status when its ingress IP changes, instead of on every reconciliation, and log it (`Service status updated`).
- Log `EIP is ready` at debug level only, as it was repeated on every reconciliation.

## [v0.1.1]
- Only update the external load balancer when its product name, VIP, subnet, endpoints or ports differ from the desired state, instead of on every reconciliation (e.g. each EndpointSlice change). Endpoints are now sent sorted.
- Upgrade the GitHub Actions to their Node.js 24 based major versions (`actions/checkout@v7`, `actions/setup-go@v7`, `golangci/golangci-lint-action@v9`, `azure/setup-helm@v5`, `docker/*` actions) and `hadolint/hadolint-action@v3.5.0`.

## [v0.1.0]
- First release of the LB controller.
