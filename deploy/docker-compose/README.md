# Docker Compose deployment

Use this deployment mode when Omni and the provider run outside Kubernetes.

## Files

- `.env.example`
- `compose.yaml`
- `provider-config.yaml.example`

## Install

```bash
cd deploy/docker-compose
cp .env.example .env
cp provider-config.yaml.example provider-config.yaml
```

Set:

- `OMNI_ENDPOINT`
- `OMNI_SERVICE_ACCOUNT_KEY`
- either `OPENNEBULA_SESSION` or `OPENNEBULA_USERNAME` plus `OPENNEBULA_PASSWORD`

Then start the provider:

```bash
docker compose up -d
```

## Notes

- The provider exposes `/metrics`, `/healthz`, and `/readyz` on port `9977`.
- `provider-config.yaml` is the main operator contract.
- `opennebula.hypervisor: auto` prefers `kvm` and falls back to `qemu`.
- `defaults.hostnameStrategy: cluster-role-sequence` is the recommended setting when the OpenNebula VM name must match the Talos hostname.
