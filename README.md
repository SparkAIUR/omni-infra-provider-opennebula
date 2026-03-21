# Omni Infrastructure Provider for OpenNebula

`omni-infra-provider-opennebula` is an Omni infrastructure provider that provisions Talos machines as OpenNebula VMs.

It is designed for operators who already run Omni and OpenNebula and want:

- deterministic VM naming so VM name == Talos hostname
- provider-managed Talos image import into OpenNebula
- policy-controlled templates, datastores, networks, and flavors
- support for both `kvm` and `qemu`
- standalone deployment with Docker Compose or in-cluster deployment with Helm

Published artifacts:

- Docker images:
  - `docker.io/nudevco/omni-infra-provider-opennebula:<tag>`
  - `ghcr.io/sparkaiur/omni-infra-provider-opennebula:<tag>`
- Helm chart: `omni-infra-provider-opennebula`

## What the provider does

The provider:

1. registers with Omni as provider `opennebula`
2. watches Omni machine requests
3. resolves the requested template, image, network, and datastore inputs
4. instantiates a Talos VM through OpenNebula
5. injects Talos/OpenNebula context so the VM enrolls into Omni
6. deprovisions the VM idempotently when Omni releases the machine

## Quick start

Choose one of the deployment paths:

- Standalone Omni lab or VM host:
  - [deploy/docker-compose/README.md](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/deploy/docker-compose/README.md)
- Omni already running in Kubernetes:
  - [helm/omni-infra-provider-opennebula/README.md](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/helm/omni-infra-provider-opennebula/README.md)

Then start with:

- public docs index: [docs/index.mdx](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/docs/index.mdx)
- getting started: [docs/getting-started.mdx](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/docs/getting-started.mdx)
- config example: [docs/examples/provider-config.mdx](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/docs/examples/provider-config.mdx)
- machine class example: [docs/examples/machineclass.mdx](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/docs/examples/machineclass.mdx)

## Operator-facing config

The runtime config is the public contract for most provider behavior. Operators can set:

- OpenNebula endpoint and template defaults
- environment profile defaults for `lab-qemu`, `mixed-staging`, `production-kvm`, or `custom`
- hypervisor mode: `auto`, `kvm`, or `qemu`
- allowed templates, datastores, and networks
- provider-side placement scoring and host preference behavior
- storage-aware placement profiles, host tags, and network zones
- provider-side preflight and manual-networking guardrails
- bootstrap profile and timing behavior
- network profiles
- flavor catalog and default flavor
- image import policy and artifact/checksum templates
- image import locking and staged cache retention
- datastore defaults
- hostname strategy
- explainability and richer provider state
- non-mutating `explain` and `support-bundle` operator commands
- observability paths and listen address

Example:

```yaml
providerID: opennebula
opennebula:
  endpoint: http://127.0.0.1:2633/RPC2
  templateName: talos-omni-base
  hypervisor: auto
  resourcePool: staging-kvm
  imageNamePattern: talos-opennebula-{{ .Arch }}-{{ .TalosVersion }}-{{ .Datastore }}-schematic-{{ .SchematicID }}
  allowedDatastores:
    - default
    - ceph-images
  allowedNetworks:
    - talos-stage-auto
    - talos-stage-manual
defaults:
  flavor: medium
  hostnameStrategy: cluster-role-sequence
environment:
  profile: mixed-staging
imageManagement:
  importOnMiss: true
  requireChecksum: true
  artifactURLTemplate: https://factory.talos.dev/image/{{ .SchematicID }}/{{ .TalosVersion }}/opennebula-{{ .Arch }}.qcow2
  checksumURLTemplate: https://factory.talos.dev/image/{{ .SchematicID }}/{{ .TalosVersion }}/opennebula-{{ .Arch }}.qcow2.sha256
placement:
  strategy: balanced
  hostTags:
    hplcsiw01:
      - local-root
    hplcsiw02:
      - ceph-rbd
  networkZones:
    staging:
      - hplcsiw01
      - hplcsiw02
policy:
  preflight:
    enabled: true
  manualNetworking:
    mode: require-validation
bootstrap:
  profile: auto
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

Hypervisor behavior:

- `auto`: inspect eligible OpenNebula hosts and prefer `kvm`, then `qemu`
- `kvm`: force `HYPERVISOR = "kvm"`
- `qemu`: force `HYPERVISOR = "qemu"`

Reliability and explainability behavior:

- provider-side preflight runs before instantiate and persists warnings/errors into provider state
- provider-side placement scoring persists the resolved host, resolved cluster, and selection reason
- placement can now enforce `storageProfile`, required/excluded host tags, and zone-aware host selection
- image resolution records whether the provider reused or imported the selected Talos artifact
- image names should include the target datastore when the same Talos artifact is imported into multiple OpenNebula datastores
- bootstrap profile selection distinguishes qemu-style lab timing from kvm-style production timing
- the provider no longer hardcodes `CPU_MODEL`; keep CPU model policy in the base OpenNebula template or environment-specific template clones
- `explain` prints a non-mutating resolution result for a providerData payload
- `support-bundle` prints a portable debug snapshot containing explain output plus live host/datastore inventory

Example operator commands:

```bash
omni-infra-provider-opennebula explain \
  --config-file ./config.yaml \
  --provider-data-file ./provider-data.yaml \
  --talos-version v1.10.0 \
  --schematic-id default

omni-infra-provider-opennebula support-bundle \
  --config-file ./config.yaml \
  --provider-data-file ./provider-data.yaml \
  --talos-version v1.10.0 \
  --schematic-id default
```

## Versioning and releases

Releases use the same version string for the Docker image and Helm chart.

- `main` publishes an edge channel:
  - `0.0.0-edge.<shortsha>`
- git tags `vX.Y.Z` publish:
  - image tag `X.Y.Z`
  - chart version `X.Y.Z`

The release automation also publishes the Helm chart index to `gh-pages`.

## Repository map

- [AGENTS.md](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/AGENTS.md): contributor and worker guidance
- [docs/](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/docs): public Mintlify-compatible documentation
- [deploy/](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/deploy): standalone deployment examples
- [helm/](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/helm): Kubernetes packaging
- [refs/KB.md](/Volumes/S0/github/_sparkai/omni-infra-provider-opennebula/refs/KB.md): tracked implementation knowledge

## Development

Core verification from the repo root:

```bash
go test ./...
uv run --project tools/ctx pytest
make omni-infra-provider-opennebula
```
