# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
- Only update the external load balancer when its product name, VIP, subnet, endpoints or ports differ from the desired state, instead of on every reconciliation (e.g. each EndpointSlice change). Endpoints are now sent sorted.
- Upgrade the GitHub Actions to their Node.js 24 based major versions (`actions/checkout@v7`, `actions/setup-go@v7`, `golangci/golangci-lint-action@v9`, `azure/setup-helm@v5`, `docker/*` actions) and `hadolint/hadolint-action@v3.5.0`.

## [v0.1.0]
- First release of the LB controller.
