# `labctl`

`labctl` is a repo-local Python CLI for rebuilding the OpenNebula staging lab from a single YAML config.

## Goals

- phase-based execution with resumability
- typed config loading and validation
- local render/audit artifacts for generated files
- reuse of existing repo-local `.assh` helpers where they already match the validated lab flow
- optional Rackspace Spot provisioning support

## Install

```bash
uv sync --project hack/labctl
```

## Usage

```bash
uv run --project hack/labctl labctl --config ~/staging-lab.yaml plan
uv run --project hack/labctl labctl --config ~/staging-lab.yaml run
uv run --project hack/labctl labctl --config ~/staging-lab.yaml phase provider
uv run --project hack/labctl labctl --config ~/staging-lab.yaml resume
```

## Commands

- `plan`: print the phase execution plan and whether each phase is enabled
- `run`: execute all enabled phases in dependency order
- `phase <name>`: execute a single phase and its unmet dependencies
- `resume`: continue from the first incomplete phase in state
- `status`: print current state from the local state file
- `validate`: rerun the validation phase
- `collect-artifacts`: collect staging artifacts from the frontend
- `destroy-handoff-cluster`: remove the `hplcsi` cluster resources
- `recreate-handoff-cluster`: rerender and recreate the `hplcsi` cluster resources

## Layout

- `src/labctl/config.py`: config models and loader
- `src/labctl/state.py`: local JSON-backed execution state
- `src/labctl/runner.py`: subprocess execution and redaction
- `src/labctl/remote.py`: `ssh` and `scp` helpers
- `src/labctl/phases/`: phase implementations
- `templates/`: rendered config/manifests written into the workspace before upload

## Notes

- Secrets stay in the operator-provided YAML config and are redacted from command logs.
- Rackspace Spot support is optional. If enabled, `labctl` expects `rsspot` to be importable in the current Python environment.
- `rsvm-omni-controller` is intentionally not a dependency of this tool.

