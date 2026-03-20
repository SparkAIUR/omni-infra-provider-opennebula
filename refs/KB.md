# Knowledge Base

## Current repository state

- The repository started as a fork-shaped copy of `omni-infra-provider-libvirt`.
- The internal design pack under `refs/docs/*.md` is the authoritative implementation baseline for the OpenNebula conversion.
- The repo originally ignored all of `refs/`; this was narrowed so tracked guidance can live beside private working material.

## Locked project decisions

- Provider ID: `opennebula`
- Go module: `github.com/SparkAIUR/omni-infra-provider-opennebula`
- Container image: `docker.io/nudevco/omni-infra-provider-opennebula:<tag>`
- Bootstrap path: Talos OpenNebula platform using OpenNebula `CONTEXT` plus Omni-generated schematic/kernel args
- OpenNebula auth: prefer `OPENNEBULA_SESSION`, otherwise require `OPENNEBULA_USERNAME` and `OPENNEBULA_PASSWORD`
- Private contextual knowledge tooling: typed Python CLI under `tools/ctx/`, managed by `uv`

## Operational reminders

- `docs/` is public. Keep private lab details out of it.
- `refs/docs/ctx/`, `refs/tasks/`, and `refs/notes/` are intentionally gitignored.
- The provider must not log raw OpenNebula auth values or rendered `USER_DATA`.

## Learned implementation details

- Keep GOCA usage isolated in `internal/pkg/opennebula/`; the provider layer should only depend on normalized refs and lifecycle methods.
- Rendered OpenNebula extra templates are safe to debug-log only after `USER_DATA` is redacted.
- The repo Makefile needed a syntax fix before binary and image targets could run successfully.
- OpenNebula image states `USED` and `USED_PERS` must be treated as reusable image states, not as import-required failures.
- OpenNebula `7.0.1` on the live lab can keep deleted VMs visible as historical `DONE` records after they disappear from active inventory; deprovision convergence must treat `DONE` as terminal.
- The single-host live lab required `onebr1` to keep `172.22.0.1/24` persistently assigned or Talos nodes would boot but never reach Omni.
- The Talos OpenNebula platform uses `SET_HOSTNAME`, not `HOSTNAME`, for hostname configuration.
- The single-host validated compatibility set is Omni `1.6.0`, Talos `1.12.4`, OpenNebula `7.0.1`, and software-emulated qemu with a `Westmere` CPU model override.
- `networkContextMode: manual` remains unvalidated on that exact lab even when the provider emits the documented Talos OpenNebula manual context.
- The staging lab rebuild automation now lives under `hack/labctl/` as a typed Python CLI with phase state persisted under `.out/labctl/.../state.json`.
- `labctl` intentionally reuses the existing `.assh` DNS, VXLAN, provider bootstrap, and artifact helpers rather than replacing them with a second shell surface.
- Rackspace Spot integration for staging rebuilds is optional and expects `rsspot` to be importable when enabled; `rsvm-omni-controller` is intentionally not part of the stack bootstrap path.

## To update as work progresses

- notable API quirks in GOCA/OpenNebula
- retry and reconcile pitfalls
- Talos bootstrap and hostname details
- CI/release gotchas
- smoke-test findings from the lab

## Context Store Digest

<!-- ctx:digest:start -->
- #4 [rule] Rendered USER_DATA must stay redacted in logs: Provisioner debug logging must redact rendered USER_DATA from OpenNebula extra templates. Keep USER_DATA_ENCODING visible, but never emit th... (tags: logging,security,user-data, source: implementation)
- #2 [decision] OpenNebula auth precedence: Prefer OPENNEBULA_SESSION when present. Otherwise require OPENNEBULA_USERNAME and OPENNEBULA_PASSWORD. (tags: auth,config,opennebula, source: implementation-plan)
- #1 [decision] Bootstrap path: Use the Talos OpenNebula platform with OpenNebula CONTEXT and minimal base64 USER_DATA; remove NoCloud/cidata from the mainline provider flo... (tags: bootstrap,talos,opennebula, source: implementation-plan)
- #5 [finding] GOCA adapter baseline: The provider uses the official GOCA client for template, image, network, datastore, VM info, and terminate operations. The adapter keeps tho... (tags: opennebula,goca,adapter, source: implementation)
- #3 [rule] Repo private paths: Keep refs/docs/ctx, refs/tasks, and refs/notes gitignored. Track refs/RULES.md, refs/KB.md, and the design pack under refs/docs. (tags: repo,gitignore,docs, source: implementation-plan)
<!-- ctx:digest:end -->
