#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  collect-production-ceph-inventory.sh --output-dir <path> [--host <host>]...

This helper gathers readonly host facts needed before Ceph automation is finalized.
Run it as a user with readonly SSH access to the production frontend and candidate hypervisors.
EOF
}

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --output-dir)
        OUTPUT_DIR="$2"
        shift 2
        ;;
      --host)
        HOSTS+=("$2")
        shift 2
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done
}

OUTPUT_DIR=""
SSH_USER="${SSH_USER:-root}"
declare -a HOSTS=()

parse_args "$@"
[[ -n "${OUTPUT_DIR}" ]] || fail "--output-dir is required"
[[ "${#HOSTS[@]}" -gt 0 ]] || fail "provide at least one --host"

mkdir -p "${OUTPUT_DIR}"

for host in "${HOSTS[@]}"; do
  log "collecting readonly inventory from ${host}"
  host_dir="${OUTPUT_DIR}/${host}"
  mkdir -p "${host_dir}"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "hostname -f || hostname")" >"${host_dir}/hostname.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "ip -br addr")" >"${host_dir}/ip-br-addr.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "ip route")" >"${host_dir}/ip-route.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "ip link")" >"${host_dir}/ip-link.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "lsblk -e7 -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT,MODEL,SERIAL")" >"${host_dir}/lsblk.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "blkid")" >"${host_dir}/blkid.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "pvs || true")" >"${host_dir}/pvs.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "vgs || true")" >"${host_dir}/vgs.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "lvs || true")" >"${host_dir}/lvs.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "systemctl list-units 'ceph*' 'opennebula*' --all")" >"${host_dir}/systemd-units.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "cephadm version || true")" >"${host_dir}/cephadm-version.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "ceph --version || true")" >"${host_dir}/ceph-version.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "ss -lntup")" >"${host_dir}/ss-lntup.txt"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "if command -v nft >/dev/null 2>&1; then nft list ruleset; else iptables-save; fi")" >"${host_dir}/firewall.txt"
done

log "readonly inventory saved under ${OUTPUT_DIR}"
