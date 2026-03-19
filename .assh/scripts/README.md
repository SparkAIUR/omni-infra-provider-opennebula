# Scripts

- `list-opennebula-resources.sh`: prints templates, images, and networks from the current OpenNebula context
- `render-provider-config.sh`: emits a provider config from the template with environment substitution
- `smoke-test.sh`: wraps a basic provider deployment and validation flow

These helpers assume operator-managed credentials and do not write secrets to the repository.
