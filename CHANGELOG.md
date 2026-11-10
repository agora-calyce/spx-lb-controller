# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]
- Fail at startup when the ProjectID cannot be discovered (no Node, missing `cluster.x-k8s.io/cluster-namespace` annotation, or value that is not a valid UUID) instead of running with an empty ProjectID.
- Migrate the build jobs from GitLab CI to GitHub Actions (`.github/workflows/build.yml`).
- Publish artifacts on GitHub instead of Harbor: image on `ghcr.io/agora-calyce/spx-lb-controller`, Helm chart on `oci://ghcr.io/agora-calyce/charts` and attached to GitHub Releases (created on tags).
- Change the chart default `image.repository` to `ghcr.io/agora-calyce/spx-lb-controller`.
- Add CI checks run before any build or release: gofmt, `go mod tidy`, `go vet`, golangci-lint, tests with race detector, `helm lint`, chart rendering, helm-docs freshness, hadolint, and tag/chart version consistency on tags.
- Regenerate the chart README (remove the obsolete `api.projectId` value).
- Align with the current SPX API: send the required `spec.subnetEId` when creating and updating load balancers, in both `internal` and `public` modes.
- Read the EIP status from the nested `eip` object (`eip.status.ready`, `eip.status.ip`) returned by the SPX API.
- Read the load balancer `subnetEId` from the nested `loadBalancer` object.
- Add a GitLab CI configuration (`.gitlab-ci.yml`) to automate building and pushing the image (via Buildah) and the Helm chart to Harbor.
- Add a Helm chart in `chart/spx-lb-controller` to deploy the controller.
- Add a Dockerfile and .dockerignore to build the controller image.
- Change the default annotations and API domain to `superphenix.net`.
- Integrate the LoadBalancer API based on the Swagger spec.
- Create the `pkg/api` package for interactions with the external API.
- Support full reconciliation of `LoadBalancer` type services.
- Handle finalizers for automatic cleanup of external resources.
- Automatically update the service status with the LoadBalancer IP.
