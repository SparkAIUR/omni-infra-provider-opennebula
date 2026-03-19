# Omni Infrastructure Provider for OpenNebula

`omni-infra-provider-opennebula` provides an OpenNebula-backed dynamic infrastructure provider for Omni. It provisions and deprovisions Talos nodes on OpenNebula, allowing Omni clusters to consume existing OpenNebula capacity through the standard Omni infrastructure-provider workflow.

## Validated live-lab path

The current validated path is a single-host lab where OpenNebula, Omni, and the provider run on the same Ubuntu host. The live run covered:

- provider registration with Omni
- control-plane bootstrap and Omni link creation
- worker scale-out
- worker scale-down
- full cluster delete and stale-VM convergence
- provider restart during provisioning without duplicate VM creation
- provider-managed Talos image import-on-miss
- image reuse on subsequent provisions
- `/healthz`, `/readyz`, `/metrics`, and auth redaction checks

Validated versions:

- Omni `1.6.0`
- Talos `1.12.4`
- OpenNebula `7.0.1`
- Ubuntu `24.04.3`

Compatibility note:

- The validated VM path used software-emulated qemu on a single host and required the Talos template CPU model to be set to `Westmere`.
- Manual networking with `networkContextMode: manual` remains a documented blocker on this exact lab combination. The provider renders the Talos-documented OpenNebula manual context correctly, but the guest still does not become reachable on the static address in this environment.

## What it does

- registers with Omni as provider `opennebula`
- resolves machine shapes from provider-managed flavors or explicit resources
- instantiates Talos VMs on OpenNebula
- bootstraps nodes through the Talos OpenNebula platform using OpenNebula `CONTEXT` plus schematic/kernel args from Omni
- keeps the Talos hostname aligned with the OpenNebula VM name
- tears down provider-owned VMs cleanly during scale-down or cluster deletion

## Repository map

- `AGENTS.md`: project entrypoint for contributors and AI workers
- `docs/`: public Mintlify-compatible project documentation
- `refs/RULES.md`: contributor rules and quality gates
- `refs/KB.md`: tracked implementation knowledge summary
- `refs/docs/`: internal architecture and implementation reference pack

## Development status

The repository is at a production-readiness hardening stage. The internal spec pack under `refs/docs/` remains the detailed implementation baseline, while the public docs now describe the validated live-lab workflow and its current compatibility envelope.

## Internal tooling

- `tools/ctx/`: private SQLite-backed context store managed with `uv`
- `.assh/`: repo-local automation scripts and templates for repeatable lab and OpenNebula workflows

## Expected image name

```text
docker.io/nudevco/omni-infra-provider-opennebula:<tag>
```

## Next references

- Start with `AGENTS.md`
- Read the public docs in `docs/`
- Follow contributor expectations in `refs/RULES.md`
- Use `docs/live-lab.mdx` for the single-host validated deployment path
