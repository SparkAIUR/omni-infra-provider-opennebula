#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NAMESPACE="${PROVIDER_NAMESPACE:-default}"

: "${OMNI_ENDPOINT:?set OMNI_ENDPOINT}"

echo "== provider deployment examples =="
echo "kubectl apply -n ${NAMESPACE} -f ${ROOT_DIR}/test/kubernetes-deployment.socket.yaml"
echo "kubectl apply -f ${ROOT_DIR}/test/machineclass.yaml"
echo
echo "== opennebula inventory =="
"${ROOT_DIR}/.assh/scripts/list-opennebula-resources.sh"
echo
echo "== next validation steps =="
echo "1. apply or update a cluster template that uses the OpenNebula machine classes"
echo "2. watch provider logs: kubectl logs -n ${NAMESPACE} deploy/omni-infra-provider-opennebula -f"
echo "3. confirm the VM exists in OpenNebula: onevm list"
echo "4. confirm the node appears in Omni via the UI or omnictl"
