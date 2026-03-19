# AGENTS

This repository builds `omni-infra-provider-opennebula`, an Omni infrastructure provider that provisions Talos nodes on OpenNebula.

## Entry Points

- Public project overview: `README.md`
- Public docs: `docs/`
- Contributor rules: `refs/RULES.md`
- Tracked implementation knowledge: `refs/KB.md`
- Internal architecture/spec pack: `refs/docs/*.md`

## Architecture Summary

The provider is a standalone Go service that:

1. registers with Omni as provider `opennebula`
2. watches dynamic machine requests
3. resolves OpenNebula template, image, datastore, and network inputs
4. instantiates a Talos VM through OpenNebula
5. bootstraps the node with minimal Omni enrollment config using OpenNebula `CONTEXT` and base64 `USER_DATA`
6. waits for the node to become available in Omni
7. deprovisions the VM idempotently when the request is released

## Worker Tracks

### `docs-and-knowledge`

Responsibilities:

- maintain `docs/` in Mintlify-compatible form
- keep `refs/KB.md` current
- capture private findings in `refs/docs/ctx/`, `refs/tasks/`, and `refs/notes/`
- keep examples synchronized with public contracts

Primary files:

- `docs/**`
- `refs/KB.md`
- `refs/RULES.md`
- `tools/ctx/**`

### `provider-core`

Responsibilities:

- runtime config and metadata
- `providerData` validation
- naming, hostname patching, bootstrap rendering
- reconcile and deprovision orchestration

Primary files:

- `cmd/omni-infra-provider-opennebula/**`
- `internal/pkg/config/**`
- `internal/pkg/provider/**`
- `api/specs/**`

### `opennebula-adapter`

Responsibilities:

- GOCA client wiring
- OpenNebula template, image, network, and VM lifecycle abstraction
- fake client for integration-style tests
- error normalization and retry semantics

Primary files:

- `internal/pkg/opennebula/**`

### `validation-and-release`

Responsibilities:

- Go and Python test coverage
- CI workflows, image build, and release plumbing
- repo-local `assh` workflows for repeatable lab actions
- smoke-test runbooks and operational handoff

Primary files:

- `.github/workflows/**`
- `Dockerfile`
- `Makefile`
- `hack/**`
- `.assh/**`

## Handoff Rules

1. Public contract changes require matching updates in `docs/` and examples.
2. Provider state-schema changes require regenerated protobuf outputs and tests.
3. Adapter changes require fake-client coverage.
4. Release changes require updated runbooks and image references.

## Implementation Sequence

1. Repo policy and private knowledge tooling
2. Entrypoint docs and repo identity
3. Repository-wide rename from libvirt to opennebula
4. Runtime config and public contracts
5. OpenNebula adapter and orchestration
6. Docs, automation, CI, and release validation

## Timeline

### Sprint 1

- governance files
- context store
- `AGENTS.md`
- `README.md`
- repo rename and build plumbing

### Sprint 2

- config
- schema
- state model
- naming/bootstrap/template rendering

### Sprint 3

- GOCA adapter
- provision/deprovision flow
- fake integration tests

### Sprint 4

- public docs and examples
- repo-local `assh` workflows
- CI and release updates
- smoke-test execution and handoff notes
