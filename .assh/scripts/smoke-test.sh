#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAMESPACE="${PROVIDER_NAMESPACE:-default}"
DEPLOY_MODE="${PROVIDER_DEPLOYMENT_MODE:-standalone}"

: "${OMNI_ENDPOINT:?set OMNI_ENDPOINT}"

echo "== provider deployment examples =="
if [[ "${DEPLOY_MODE}" == "kubernetes" ]]; then
  echo "kubectl apply -n ${NAMESPACE} -f ${ROOT_DIR}/test/kubernetes-deployment.socket.yaml"
  echo "kubectl apply -f ${ROOT_DIR}/test/machineclass.yaml"
else
  echo "docker compose -f /opt/omni-provider-opennebula/compose.yaml up -d provider"
  echo "${ROOT_DIR}/.assh/scripts/render-provider-config.sh > /tmp/omni-provider-config.yaml"
  echo "omnictl apply -f ${ROOT_DIR}/test/machineclass.yaml"
fi
echo
echo "== opennebula inventory =="
"${ROOT_DIR}/.assh/scripts/list-opennebula-resources.sh"
echo
echo "== next validation steps =="
echo "1. apply or update a cluster template that uses the OpenNebula machine classes"
if [[ "${DEPLOY_MODE}" == "kubernetes" ]]; then
  echo "2. watch provider logs: kubectl logs -n ${NAMESPACE} deploy/omni-infra-provider-opennebula -f"
else
  echo "2. watch provider logs: docker logs -f omni-infra-provider-opennebula"
  echo "3. check health: curl -sf http://127.0.0.1:9977/healthz && curl -sf http://127.0.0.1:9977/readyz"
fi
echo "4. confirm the VM exists in OpenNebula: onevm list"
echo "5. confirm the node appears in Omni via the UI or omnictl"
