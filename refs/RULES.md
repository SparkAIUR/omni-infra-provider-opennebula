# Repository Rules

## Scope

This repository implements the OpenNebula infrastructure provider for Omni. The codebase is primarily Go, with a small typed Python utility under `tools/ctx/` for private contextual knowledge management.

## Contribution Rules

1. Keep the provider runtime and public contracts strongly typed.
2. Prefer small, atomic commits aligned to one logical change.
3. Preserve the upstream MPL license headers in Go source files.
4. Do not commit secrets, OpenNebula credentials, service-account keys, or lab-specific sensitive output.
5. Treat `docs/` as public project documentation and `refs/` as contributor-facing internal guidance.

## Directory Policy

- `docs/`: public Mintlify-compatible project documentation.
- `refs/RULES.md`: tracked contributor and process rules.
- `refs/KB.md`: tracked distilled knowledge and recurring lessons.
- `refs/docs/*.md`: tracked implementation and architecture reference pack.
- `refs/docs/ctx/`: private SQLite context store and transient indexes. Gitignored.
- `refs/tasks/`: private execution plans, scratch plans, and agent notes. Gitignored.
- `refs/notes/`: private experiments and sensitive findings. Gitignored.

## Go Standards

1. All exported types and functions must have doc comments.
2. Keep top-level classes, interfaces, and high-usage functions documented with short usage examples where practical.
3. Keep functions cohesive. Split orchestration, rendering, validation, and transport concerns into separate packages/files.
4. Prefer explicit structs and enums-like constants over `map[string]any`.
5. Keep logging structured and redact secrets, `USER_DATA`, session strings, and passwords.
6. Avoid platform-specific assumptions outside the OpenNebula adapter package.

## Python Standards

1. Use `uv` for dependency and environment management.
2. Keep the Python surface limited to internal tooling under `tools/ctx/`.
3. Use type annotations throughout and keep `mypy` clean.
4. Prefer the standard library unless a dependency materially improves correctness or maintainability.

## Testing Requirements

Before merging a meaningful change, run the tests that cover the touched surface:

- Go unit and integration-style tests: `go test ./...`
- Provider build verification: `make omni-infra-provider-opennebula`
- Context CLI tests: `uv run --project tools/ctx pytest`
- Context CLI static checks when touched: `uv run --project tools/ctx ruff check .` and `uv run --project tools/ctx mypy src`

For release candidates, also complete the documented smoke workflow against a real Omni/OpenNebula lab.

## Documentation Rules

1. Update `docs/` whenever public behavior, configuration, deployment, or contributor workflows change.
2. Keep examples copy-paste ready and synchronized with the implemented schema.
3. Record notable implementation decisions, bugs, and operational lessons in `refs/KB.md`.
4. Put non-public lab findings in `refs/notes/` or the private context store, not in `docs/`.

## Review Checklist

Every substantive change should be reviewed for:

- public contract changes
- backward-compatibility impact
- secret handling and log redaction
- retry/idempotency behavior
- test coverage
- docs updates

## Commit Hygiene

Recommended commit sequence for larger work:

1. repo scaffolding and governance
2. rename/build plumbing
3. contracts and schemas
4. adapter and provider logic
5. docs/examples/automation
6. test and CI follow-up

Keep generated files committed when they are part of the repo contract, including protobuf outputs, docs navigation, and example artifacts.
