# Knowledge Base

## Current repository state

- The repository started as a fork-shaped copy of `omni-infra-provider-libvirt`.
- The internal design pack under `refs/docs/*.md` is the authoritative implementation baseline for the OpenNebula conversion.
- The repo originally ignored all of `refs/`; this was narrowed so tracked guidance can live beside private working material.

## Locked project decisions

- Provider ID: `opennebula`
- Go module: `github.com/SparkAIUR/omni-infra-provider-opennebula`
- Container image: `docker.io/nudevco/omni-infra-provider-opennebula:<tag>`
- Bootstrap path: Talos OpenNebula platform using `CONTEXT` plus minimal base64 `USER_DATA`
- OpenNebula auth: prefer `OPENNEBULA_SESSION`, otherwise require `OPENNEBULA_USERNAME` and `OPENNEBULA_PASSWORD`
- Private contextual knowledge tooling: typed Python CLI under `tools/ctx/`, managed by `uv`

## Operational reminders

- `docs/` is public. Keep private lab details out of it.
- `refs/docs/ctx/`, `refs/tasks/`, and `refs/notes/` are intentionally gitignored.
- The provider must not log raw OpenNebula auth values or rendered `USER_DATA`.

## To update as work progresses

- notable API quirks in GOCA/OpenNebula
- retry and reconcile pitfalls
- Talos bootstrap and hostname details
- CI/release gotchas
- smoke-test findings from the lab

## Context Store Digest

<!-- ctx:digest:start -->
- #2 [decision] OpenNebula auth precedence: Prefer OPENNEBULA_SESSION when present. Otherwise require OPENNEBULA_USERNAME and OPENNEBULA_PASSWORD. (tags: auth,config,opennebula, source: implementation-plan)
- #1 [decision] Bootstrap path: Use the Talos OpenNebula platform with OpenNebula CONTEXT and minimal base64 USER_DATA; remove NoCloud/cidata from the mainline provider flo... (tags: bootstrap,talos,opennebula, source: implementation-plan)
- #3 [rule] Repo private paths: Keep refs/docs/ctx, refs/tasks, and refs/notes gitignored. Track refs/RULES.md, refs/KB.md, and the design pack under refs/docs. (tags: repo,gitignore,docs, source: implementation-plan)
<!-- ctx:digest:end -->
