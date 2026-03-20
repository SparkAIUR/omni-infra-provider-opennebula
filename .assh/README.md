# Repo-local `assh` helpers

This directory holds repo-specific automation and templates for repeated OpenNebula and smoke-test workflows.

The helpers were used for the single-host live lab where OpenNebula, Omni, and the provider ran on the same machine, and now also include staging-lab bootstrap wrappers for the multi-host `on.lab.sprkinfra.com` environment.

## Layout

- `scripts/`: shell helpers for listing resources and running smoke flows
- `templates/`: reusable config snippets and placeholders

Local `config.toml`, `state.db`, `logs/`, and `sockets/` are runtime state and should remain ignored.

## Expected environment

- OpenNebula CLI tools such as `onetemplate`, `oneimage`, `onevnet`, and `onevm`
- Docker for standalone provider deployment
- `omnictl` for Omni-side validation when needed

## Supported environment variables

- `ONE_ENDPOINT`
- `ONE_AUTH`
- `OMNI_ENDPOINT`
- `OMNI_SERVICE_ACCOUNT_KEY`
- `PROVIDER_NAMESPACE`

The scripts do not persist credentials. Export them in your shell or source them from your preferred secret-management flow.

## Recommended usage

1. Use `bootstrap-dns.sh` to publish the staging-lab frontend records.
2. Use `bootstrap-vxlan.sh` to build the Talos guest overlay on each node.
   On the frontend, pass `VXLAN_GATEWAY_IP` so the helper also enables NAT and TCP MSS clamping for guest egress.
3. Use `render-provider-config.sh` to produce a provider config from the tracked template.
4. Use `bootstrap-provider.sh` to deploy the standalone provider container on the frontend.
5. Use `run-provider-e2e.sh` and `collect-staging-artifacts.sh` to validate and capture the staging state.
6. Use `list-opennebula-resources.sh` and `smoke-test.sh` for the existing single-host flow when needed.
