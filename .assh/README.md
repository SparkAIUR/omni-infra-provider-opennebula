# Repo-local `assh` helpers

This directory holds repo-specific automation and templates for repeated OpenNebula and smoke-test workflows.

## Layout

- `scripts/`: shell helpers for listing resources and running smoke flows
- `templates/`: reusable config snippets and placeholders

Local `config.toml`, `state.db`, `logs/`, and `sockets/` are runtime state and should remain ignored.

## Expected environment

- OpenNebula CLI tools such as `onetemplate`, `oneimage`, `onevnet`, and `onevm`
- `kubectl`
- `omnictl` for Omni-side validation when needed

## Supported environment variables

- `ONE_ENDPOINT`
- `ONE_AUTH`
- `OMNI_ENDPOINT`
- `OMNI_SERVICE_ACCOUNT_KEY`
- `PROVIDER_NAMESPACE`

The scripts do not persist credentials. Export them in your shell or source them from your preferred secret-management flow.
