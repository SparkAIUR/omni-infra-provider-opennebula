# Scripts

- `list-opennebula-resources.sh`: prints templates, images, and networks from the current OpenNebula context
- `render-provider-config.sh`: emits a provider config from the template with environment substitution
- `smoke-test.sh`: wraps the single-host live-lab validation flow
- `bootstrap-dns.sh`: creates or updates the staging-lab Cloudflare A records
- `bootstrap-vxlan.sh`: configures the Talos VXLAN bridge on a target node, including frontend NAT and TCP MSS clamping when a gateway IP is provided
- `bootstrap-provider.sh`: deploys the standalone provider container on a frontend
- `run-provider-e2e.sh`: checks provider health, metrics, and base inventory
- `collect-staging-artifacts.sh`: snapshots frontend service and OpenNebula state into a private artifact directory
- `collect-production-ceph-inventory.sh`: gathers readonly host facts for production Ceph planning
- `setup-production-ceph-storage.sh`: renders, reviews, and optionally executes the additive production Ceph and datastore setup flow

These helpers assume operator-managed credentials and do not write secrets to the repository.

`smoke-test.sh` defaults to the validated standalone-container flow. Set `PROVIDER_DEPLOYMENT_MODE=kubernetes` if you want the older in-cluster example output instead.

The staging-lab scripts are intentionally thin wrappers around SSH-accessible targets. They assume operator-managed credentials and a root-capable frontend.
