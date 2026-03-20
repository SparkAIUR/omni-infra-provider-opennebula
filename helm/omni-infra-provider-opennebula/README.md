# omni-infra-provider-opennebula Helm Chart

This chart deploys the OpenNebula Omni infrastructure provider into an existing Kubernetes cluster.

It installs:

- the provider Deployment
- the provider ServiceAccount
- a ConfigMap containing `provider-config.yaml`
- optional Secret objects for inline OpenNebula and Omni credentials
- a Service for `/metrics`, `/healthz`, and `/readyz`
- an optional ServiceMonitor

Published images for this chart:

- `docker.io/nudevco/omni-infra-provider-opennebula:<tag>`
- `ghcr.io/sparkaiur/omni-infra-provider-opennebula:<tag>`

Chart repo:

- `https://sparkaiur.github.io/omni-infra-provider-opennebula/charts/`

## What This Chart Supports

- deploying the provider into an existing Omni-managed Kubernetes cluster
- OpenNebula authentication from an existing Secret or inline Helm value
- Omni service-account authentication from an existing Secret or inline Helm value
- provider-side defaults for templates, flavors, image import, observability, naming, and OpenNebula placement
- metrics scraping through a normal Service or Prometheus Operator ServiceMonitor

## Prerequisites

- a reachable Omni API endpoint
- a reachable OpenNebula XML-RPC endpoint
- an Omni service account key
- OpenNebula credentials for the provider
- OpenNebula-side templates, networks, and datastores prepared for the machine classes you intend to expose

## Install

Add the chart repo:

```bash
helm repo add sparkai-opennebula https://sparkaiur.github.io/omni-infra-provider-opennebula/charts/
helm repo update
```

### Install with Existing Secrets

Create the required secrets:

```bash
kubectl -n omni-system create secret generic omni-provider-opennebula-auth \
  --from-literal=credentials='oneadmin:changeme'

kubectl -n omni-system create secret generic omni-provider-opennebula-omni \
  --from-literal=key='<omni-service-account-key>'
```

Install the chart:

```bash
helm upgrade --install omni-infra-provider-opennebula sparkai-opennebula/omni-infra-provider-opennebula \
  --namespace omni-system \
  --create-namespace \
  --set omni.endpoint=https://omni.example.com \
  --set credentials.existingSecret.name=omni-provider-opennebula-auth \
  --set omni.serviceAccount.existingSecret.name=omni-provider-opennebula-omni
```

### Install with Inline Credentials

```bash
helm upgrade --install omni-infra-provider-opennebula sparkai-opennebula/omni-infra-provider-opennebula \
  --namespace omni-system \
  --create-namespace \
  --set omni.endpoint=https://omni.example.com \
  --set credentials.inlineAuth='oneadmin:changeme' \
  --set omni.serviceAccount.inlineKey='<omni-service-account-key>'
```

Inline credentials are supported for convenience. Existing Secrets are the preferred production path.

## Typical Configuration Patterns

### Existing Secrets with Automatic Hypervisor Selection

```yaml
omni:
  endpoint: https://omni.example.com
  serviceAccount:
    existingSecret:
      name: omni-provider-opennebula-omni
      key: key

credentials:
  existingSecret:
    name: omni-provider-opennebula-auth
    key: credentials

providerConfig:
  opennebula:
    endpoint: http://127.0.0.1:2633/RPC2
    templateName: talos-omni-base
    hypervisor: auto
```

`hypervisor: auto` prefers `kvm` and falls back to `qemu`.

### Deterministic Hostnames for OpenNebula-backed Workers

```yaml
providerConfig:
  defaults:
    hostnameStrategy: cluster-role-sequence
    networkContextMode: auto
```

Use `cluster-role-sequence` when the OpenNebula VM name must match the Talos hostname.

### Custom Flavor Presets

```yaml
providerConfig:
  defaults:
    flavor: medium
  flavors:
    small:
      cpu: "2"
      vcpu: 2
      memoryMiB: 4096
      rootDiskGiB: 40
    medium:
      cpu: "4"
      vcpu: 4
      memoryMiB: 8192
      rootDiskGiB: 60
```

### Factory Image Import with Checksum Validation

```yaml
providerConfig:
  imageManagement:
    importOnMiss: true
    requireChecksum: true
    artifactURLTemplate: https://factory.talos.dev/image/{{ .SchematicID }}/{{ .TalosVersion }}/opennebula-{{ .Arch }}.qcow2
    checksumURLTemplate: https://factory.talos.dev/image/{{ .SchematicID }}/{{ .TalosVersion }}/opennebula-{{ .Arch }}.qcow2.sha256
```

## Values Reference

`Required` meanings:

- `No`: a safe default exists
- `Yes`: must be set
- `Conditional`: required only when the related feature or deployment pattern is used

### Global Values

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `nameOverride` | Override the chart name portion used for generated resource names. | `""` | No |
| `fullnameOverride` | Override the full generated release name. | `""` | No |
| `namespaceOverride` | Override the namespace used by rendered resources. | `""` | No |
| `replicaCount` | Number of provider replicas in the Deployment. | `1` | No |
| `imagePullSecrets` | Image pull secrets attached to the provider pod. | `[]` | No |

### Image

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `image.repository` | Provider image repository. Both Docker Hub and GHCR images are published. | `docker.io/nudevco/omni-infra-provider-opennebula` | No |
| `image.tag` | Provider image tag. | `0.1.0` | No |
| `image.pullPolicy` | Image pull policy for the provider container. | `IfNotPresent` | No |

### Omni

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `omni.endpoint` | Omni API endpoint passed to `--omni-api-endpoint`. | `https://omni.example.com` | Yes |
| `omni.serviceAccount.existingSecret.name` | Name of an existing Secret containing the Omni service-account key. | `""` | Conditional |
| `omni.serviceAccount.existingSecret.key` | Secret key inside `omni.serviceAccount.existingSecret.name` used for `OMNI_SERVICE_ACCOUNT_KEY`. | `key` | No |
| `omni.serviceAccount.inlineKey` | Inline Omni service-account key used to render a Secret from the chart. | `""` | Conditional |

One of `omni.serviceAccount.existingSecret.name` or `omni.serviceAccount.inlineKey` must be set.

### OpenNebula Credentials

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `credentials.existingSecret.name` | Name of an existing Secret containing OpenNebula credentials. | `""` | Conditional |
| `credentials.existingSecret.key` | Secret key inside `credentials.existingSecret.name` used for the combined `OPENNEBULA_SESSION` value. | `credentials` | No |
| `credentials.inlineAuth` | Inline `username:password` credentials used to render a Secret from the chart. | `""` | Conditional |

One of `credentials.existingSecret.name` or `credentials.inlineAuth` must be set.

### Provider Config

The chart renders `providerConfig` directly into `provider-config.yaml`. These fields control the provider runtime contract rather than Kubernetes resources.

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `providerConfig.providerID` | Provider ID registered with Omni. | `opennebula` | No |
| `providerConfig.opennebula.endpoint` | OpenNebula XML-RPC endpoint used by the provider. | `http://127.0.0.1:2633/RPC2` | Yes |
| `providerConfig.opennebula.templateName` | Base OpenNebula template name used for VM instantiation. | `talos-omni-base` | Yes |
| `providerConfig.opennebula.hypervisor` | Hypervisor mode. Supported values: `auto`, `kvm`, `qemu`. | `auto` | No |
| `providerConfig.opennebula.resourcePool` | Optional OpenNebula cluster or resource-pool scope used for host selection. | `staging-kvm` | No |
| `providerConfig.defaults.flavor` | Default flavor name used when machine requests do not override sizing. | `medium` | Conditional |
| `providerConfig.defaults.firmware` | Firmware type used for created VMs. | `uefi` | No |
| `providerConfig.defaults.secureBoot` | Enable secure boot in rendered VM templates. | `false` | No |
| `providerConfig.defaults.graphicsEnabled` | Enable graphics devices in rendered VM templates. | `false` | No |
| `providerConfig.defaults.networkContextMode` | Network context rendering mode. | `auto` | No |
| `providerConfig.defaults.hostnameStrategy` | VM naming and hostname strategy. | `cluster-role-sequence` | No |
| `providerConfig.imageManagement.importOnMiss` | Import Talos images into OpenNebula when the requested image is missing. | `true` | No |
| `providerConfig.imageManagement.requireChecksum` | Require checksum verification for imported Talos images. | `true` | No |
| `providerConfig.imageManagement.artifactURLTemplate` | Artifact URL template for Talos OpenNebula images. | `https://factory.talos.dev/image/{{ .SchematicID }}/{{ .TalosVersion }}/opennebula-{{ .Arch }}.qcow2` | Conditional |
| `providerConfig.imageManagement.checksumURLTemplate` | Checksum URL template paired with `artifactURLTemplate`. | `https://factory.talos.dev/image/{{ .SchematicID }}/{{ .TalosVersion }}/opennebula-{{ .Arch }}.qcow2.sha256` | Conditional |
| `providerConfig.observability.listenAddress` | Provider listen address for metrics and health endpoints. | `:9977` | No |
| `providerConfig.observability.metricsPath` | Metrics path served by the provider. | `/metrics` | No |
| `providerConfig.observability.healthPath` | Liveness endpoint path served by the provider. | `/healthz` | No |
| `providerConfig.observability.readyPath` | Readiness endpoint path served by the provider. | `/readyz` | No |
| `providerConfig.flavors` | Named flavor map rendered into provider configuration. | see `values.yaml` | Conditional |
| `providerConfig.flavors.<name>.cpu` | OpenNebula `CPU` value rendered for that flavor. | none | Conditional |
| `providerConfig.flavors.<name>.vcpu` | OpenNebula `VCPU` count rendered for that flavor. | none | Conditional |
| `providerConfig.flavors.<name>.memoryMiB` | Memory size in MiB for that flavor. | none | Conditional |
| `providerConfig.flavors.<name>.rootDiskGiB` | Root disk size in GiB for that flavor. | none | Conditional |

At least one flavor must exist if you reference `providerConfig.defaults.flavor`.

### Pod Runtime

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `resources` | Pod resource requests and limits for the provider container. | `{}` | No |
| `nodeSelector` | Node selector for the provider Deployment. | `{}` | No |
| `tolerations` | Tolerations for the provider Deployment. | `[]` | No |
| `affinity` | Affinity rules for the provider Deployment. | `{}` | No |
| `podAnnotations` | Extra annotations added to the provider pod template. | `{}` | No |
| `podLabels` | Extra labels added to the provider pod template. | `{}` | No |

### Service

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `service.type` | Kubernetes Service type for provider observability endpoints. | `ClusterIP` | No |
| `service.port` | Service port exposed for the provider observability listener. | `9977` | No |
| `service.annotations` | Extra annotations for the Service. | `{}` | No |
| `service.labels` | Extra labels for the Service. | `{}` | No |

Keep `service.port` aligned with the port portion of `providerConfig.observability.listenAddress`.

### ServiceAccount

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `serviceAccount.create` | Create a dedicated Kubernetes ServiceAccount for the provider pod. | `true` | No |
| `serviceAccount.annotations` | Extra annotations for the ServiceAccount. | `{}` | No |
| `serviceAccount.name` | Existing or custom ServiceAccount name. When empty, the chart generates one. | `""` | No |

### Metrics

| Parameter | Description | Default | Required |
| --- | --- | --- | --- |
| `metrics.serviceMonitor.enabled` | Create a Prometheus Operator ServiceMonitor for provider metrics. | `false` | No |
| `metrics.serviceMonitor.namespace` | Namespace where the ServiceMonitor is created. Empty means the release namespace. | `""` | No |
| `metrics.serviceMonitor.interval` | Prometheus scrape interval. | `30s` | No |
| `metrics.serviceMonitor.scrapeTimeout` | Prometheus scrape timeout. | `10s` | No |
| `metrics.serviceMonitor.labels` | Extra labels for the ServiceMonitor. | `{}` | No |
| `metrics.serviceMonitor.annotations` | Extra annotations for the ServiceMonitor. | `{}` | No |

### Notes

- `providerConfig.opennebula.hypervisor: auto` prefers `kvm` and falls back to `qemu`.
- `providerConfig.defaults.hostnameStrategy: cluster-role-sequence` is the recommended setting when the OpenNebula VM name must match the Talos hostname.
- Existing Secrets are the preferred production path for both Omni and OpenNebula credentials.
- `providerConfig.observability.metricsPath`, `healthPath`, and `readyPath` are wired into the Deployment probes and ServiceMonitor.
- `service.port` must match the port configured in `providerConfig.observability.listenAddress`.
