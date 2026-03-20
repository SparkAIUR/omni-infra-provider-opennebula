#!/usr/bin/env bash
set -euo pipefail

: "${TARGET:?set TARGET}"

printf '== provider health ==\n'
ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "curl -sf http://127.0.0.1:9977/healthz && echo && curl -sf http://127.0.0.1:9977/readyz && echo"
printf '\n== provider metrics head ==\n'
ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "curl -sf http://127.0.0.1:9977/metrics | sed -n '1,80p'"
printf '\n== opennebula inventory ==\n'
ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "sudo -u oneadmin -H onehost list && echo && sudo -u oneadmin -H onevm list"
