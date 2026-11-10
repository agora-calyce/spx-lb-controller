# spx-lb-controller

![Version: 0.1.0](https://img.shields.io/badge/Version-0.1.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.0](https://img.shields.io/badge/AppVersion-0.1.0-informational?style=flat-square)

A Helm chart for Kubernetes LoadBalancer controller for SPX KaaS

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Sébastien SCHMITT | <sebastien.schmitt@agoracalyce.com> |  |

## Requirements

Kubernetes: `>=1.33.0-0`

## Values

### API Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| api.az | string | `""` | AZ Code (lowercase) |
| api.existingSecret | string | `""` | Optional: reference an existing secret for the configuration If existingSecret is provided, url, token, orgaId and az from values are ignored |
| api.orgaId | string | `""` | Organisation ID |
| api.token | string | `""` | API endpoint access token |
| api.url | string | `"https://console.agoracalyce.com/api"` | API endpoint base url |

### Controller settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controller.debug | bool | `false` | True to display all api requests/response |
| controller.leaderElect | bool | `false` | Ensure a leader is elected, mandatory to have more than one replicas |
| controller.logLevel | string | `"info"` | Log level for the controller |
| controller.metricsAddr | string | `":8080"` | Metrics port |
| controller.probeAddr | string | `":8081"` | Probes port |

### Extras

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| extraSecrets | list | `[]` | Extra secrets to create |

### Service account

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| serviceAccount.annotations | object | `{}` | Annotations to add to the service account |
| serviceAccount.create | bool | `true` | Specifies whether a service account should be created |
| serviceAccount.name | string | `""` | The name of the service account to use. If not set and create is true, a name is generated using the fullname template |

### Other Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` |  |
| fullnameOverride | string | `""` |  |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/agora-calyce/spx-lb-controller","tag":""}` | Image config |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy |
| image.repository | string | `"ghcr.io/agora-calyce/spx-lb-controller"` | Image to use for the controller |
| image.tag | string | `""` | Overrides the image tag whose default is the chart appVersion. |
| imagePullSecrets | list | `[]` | Image pull secrets if private repositiry is used |
| metrics.service.port | int | `8080` |  |
| metrics.service.type | string | `"ClusterIP"` |  |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| podSecurityContext.runAsNonRoot | bool | `true` |  |
| replicaCount | int | `1` | To use multiple replicas, ensure `.controller.leaderElect: true` is set |
| resources.limits.cpu | string | `"100m"` |  |
| resources.limits.memory | string | `"128Mi"` |  |
| resources.requests.cpu | string | `"100m"` |  |
| resources.requests.memory | string | `"128Mi"` |  |
| securityContext.allowPrivilegeEscalation | bool | `false` |  |
| securityContext.capabilities.drop[0] | string | `"ALL"` |  |
| securityContext.readOnlyRootFilesystem | bool | `true` |  |
| securityContext.runAsGroup | int | `65532` |  |
| securityContext.runAsUser | int | `65532` |  |
| tolerations | list | `[]` |  |

## Dev notes

Developing this chart is done using helm plugins

### Helm doc

- https://github.com/norwoodj/helm-docs/tree/master

This plugin allo to generate this readme using th `helm-docs .` command. 
The doc can be configured by settings comments and @ keywords in the values.yaml file.
