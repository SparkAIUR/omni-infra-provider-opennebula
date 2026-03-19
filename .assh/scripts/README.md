# Scripts

- `list-opennebula-resources.sh`: prints templates, images, and networks from the current OpenNebula context
- `render-provider-config.sh`: emits a provider config from the template with environment substitution
- `smoke-test.sh`: wraps the single-host live-lab validation flow

These helpers assume operator-managed credentials and do not write secrets to the repository.

`smoke-test.sh` defaults to the validated standalone-container flow. Set `PROVIDER_DEPLOYMENT_MODE=kubernetes` if you want the older in-cluster example output instead.
