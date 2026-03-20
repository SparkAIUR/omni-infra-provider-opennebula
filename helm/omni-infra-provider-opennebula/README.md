# omni-infra-provider-opennebula Helm Chart

This chart deploys the OpenNebula Omni infrastructure provider into an existing Kubernetes cluster.

Use this chart when Omni already runs in Kubernetes and you want the provider deployed as a normal workload.

## Installs

- Deployment
- ServiceAccount
- ConfigMap
- Secret objects for inline credentials
- Service for `/metrics`, `/healthz`, and `/readyz`
- optional ServiceMonitor

## Install

Add the chart repo:

```bash
helm repo add sparkai-opennebula https://sparkaiur.github.io/omni-infra-provider-opennebula/charts/
helm repo update
```

### Existing secrets

Create the required secrets:

```bash
kubectl create secret generic omni-provider-opennebula-auth \
  --from-literal=credentials='oneadmin:changeme'

kubectl create secret generic omni-provider-opennebula-omni \
  --from-literal=key='<omni-service-account-key>'
```

Install:

```bash
helm upgrade --install omni-infra-provider-opennebula sparkai-opennebula/omni-infra-provider-opennebula \
  --namespace omni-system \
  --create-namespace \
  --set omni.endpoint=https://omni.example.com \
  --set credentials.existingSecret.name=omni-provider-opennebula-auth \
  --set omni.serviceAccount.existingSecret.name=omni-provider-opennebula-omni
```

### Inline secrets

```bash
helm upgrade --install omni-infra-provider-opennebula ./helm/omni-infra-provider-opennebula \
  --namespace omni-system \
  --create-namespace \
  --set omni.endpoint=https://omni.example.com \
  --set credentials.inlineAuth='oneadmin:changeme' \
  --set omni.serviceAccount.inlineKey='<omni-service-account-key>'
```

## Important values

| Parameter | Description | Default |
| --- | --- | --- |
| `image.repository` | Provider image repository | `docker.io/nudevco/omni-infra-provider-opennebula` |
| `image.tag` | Provider image tag | `0.1.0` |
| `omni.endpoint` | Omni API endpoint passed to the provider | `https://omni.example.com` |
| `providerConfig.opennebula.endpoint` | OpenNebula XML-RPC endpoint | `http://127.0.0.1:2633/RPC2` |
| `providerConfig.opennebula.hypervisor` | `auto`, `kvm`, or `qemu` | `auto` |
| `providerConfig.defaults.hostnameStrategy` | VM naming strategy | `cluster-role-sequence` |
| `providerConfig.imageManagement.artifactURLTemplate` | Talos OpenNebula image artifact template | factory `qcow2` URL |

## Notes

- `opennebula.hypervisor: auto` prefers `kvm` and falls back to `qemu`.
- `cluster-role-sequence` is the recommended hostname strategy when the OpenNebula VM name must match the Talos hostname.
- Use an existing Secret for production instead of inline values.
