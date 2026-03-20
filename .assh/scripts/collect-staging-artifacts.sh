#!/usr/bin/env bash
set -euo pipefail

: "${TARGET:?set TARGET}"

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
out="${OUT_DIR:-refs/tasks/staging-lab/${stamp}/artifacts}"
mkdir -p "${out}"

ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "bash -lc '
  echo \"== ceph ==\"
  ceph -s || true
  echo
  echo \"== opennebula hosts ==\"
  sudo -u oneadmin -H onehost list || true
  echo
  echo \"== opennebula datastores ==\"
  sudo -u oneadmin -H onedatastore list || true
  echo
  echo \"== opennebula templates ==\"
  sudo -u oneadmin -H onetemplate list || true
  echo
  echo \"== opennebula vms ==\"
  sudo -u oneadmin -H onevm list || true
  echo
  echo \"== docker ps ==\"
  docker ps || true
' " > "${out}/frontend-summary.txt"

scp -o StrictHostKeyChecking=accept-new "root@${TARGET}:/var/log/one/oned.log" "${out}/oned.log" >/dev/null 2>&1 || true
scp -o StrictHostKeyChecking=accept-new "root@${TARGET}:${PROVIDER_LOG_PATH:-/opt/omni-provider-opennebula/provider.log}" "${out}/provider.log" >/dev/null 2>&1 || true

printf '%s\n' "${out}"
