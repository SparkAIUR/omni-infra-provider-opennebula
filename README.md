# Omni Infrastructure Provider for OpenNebula

`omni-infra-provider-opennebula` provides an OpenNebula-backed dynamic infrastructure provider for Omni. It provisions and deprovisions Talos nodes on OpenNebula, allowing Omni clusters to consume existing OpenNebula capacity through the standard Omni infrastructure-provider workflow.

## What it does

- registers with Omni as provider `opennebula`
- resolves machine shapes from provider-managed flavors or explicit resources
- instantiates Talos VMs on OpenNebula
- bootstraps nodes through the Talos OpenNebula platform using OpenNebula `CONTEXT` and minimal base64 `USER_DATA`
- keeps the Talos hostname aligned with the OpenNebula VM name
- tears down provider-owned VMs cleanly during scale-down or cluster deletion

## Repository map

- `AGENTS.md`: project entrypoint for contributors and AI workers
- `docs/`: public Mintlify-compatible project documentation
- `refs/RULES.md`: contributor rules and quality gates
- `refs/KB.md`: tracked implementation knowledge summary
- `refs/docs/`: internal architecture and implementation reference pack

## Development status

The repository is under active v1 delivery. The internal spec pack under `refs/docs/` defines the target architecture, integration strategy, testing expectations, and deployment handoff.

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
