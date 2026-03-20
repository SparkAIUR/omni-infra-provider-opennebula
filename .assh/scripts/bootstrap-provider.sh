#!/usr/bin/env bash
set -euo pipefail

: "${TARGET:?set TARGET}"
: "${OMNI_ENDPOINT:?set OMNI_ENDPOINT}"
: "${OMNI_SERVICE_ACCOUNT_KEY:?set OMNI_SERVICE_ACCOUNT_KEY}"
: "${OPENNEBULA_USERNAME:?set OPENNEBULA_USERNAME}"
: "${OPENNEBULA_PASSWORD:?set OPENNEBULA_PASSWORD}"

provider_root="${PROVIDER_ROOT:-/opt/omni-provider-opennebula}"
config_path="${PROVIDER_CONFIG_PATH:-${provider_root}/config.yaml}"
env_path="${PROVIDER_ENV_PATH:-${provider_root}/provider.env}"
config_source="${PROVIDER_CONFIG_SOURCE:-}"
image="${PROVIDER_IMAGE:-docker.io/nudevco/omni-infra-provider-opennebula:latest}"
staging_dir="${PROVIDER_STAGING_DIR:-/var/tmp/omni-infra-provider-opennebula}"
insecure_skip_verify="${PROVIDER_INSECURE_SKIP_VERIFY:-false}"

ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "mkdir -p '${provider_root}'"

if [[ -n "${config_source}" ]]; then
  scp -o StrictHostKeyChecking=accept-new "${config_source}" "root@${TARGET}:${config_path}" >/dev/null
fi

ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "cat > '${env_path}'" <<EOF
OMNI_ENDPOINT=${OMNI_ENDPOINT}
OMNI_SERVICE_ACCOUNT_KEY=${OMNI_SERVICE_ACCOUNT_KEY}
OPENNEBULA_USERNAME=${OPENNEBULA_USERNAME}
OPENNEBULA_PASSWORD=${OPENNEBULA_PASSWORD}
EOF

cat <<EOF | ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "cat > '${provider_root}/compose.yaml'"
services:
  provider:
    image: ${image}
    container_name: omni-infra-provider-opennebula
    restart: unless-stopped
    network_mode: host
    env_file:
      - ${env_path}
    volumes:
      - ${config_path}:/etc/omni-provider/config.yaml:ro
      - ${staging_dir}:${staging_dir}
    command:
      - /omni-infra-provider-opennebula
      - --config-file
      - /etc/omni-provider/config.yaml
$(if [[ "${insecure_skip_verify}" == "true" ]]; then printf '%s\n' "      - --insecure-skip-verify"; fi)
EOF

ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "mkdir -p '${staging_dir}' && (docker image inspect '${image}' >/dev/null 2>&1 || docker pull '${image}')"
ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "docker compose -f '${provider_root}/compose.yaml' up -d"
