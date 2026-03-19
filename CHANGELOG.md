# Changelog

All notable changes to `omni-infra-provider-opennebula` will be documented in this file.

## Unreleased

- OpenNebula provider identity, Docker image target, and release metadata aligned on `docker.io/nudevco/omni-infra-provider-opennebula:<tag>`.
- CI now validates the private `tools/ctx` helper alongside Go build, lint, and unit-test flows.
- Example deployment manifests now use the OpenNebula runtime config contract and supported auth environment variables.
