#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEMPLATE_DIR="${REPO_ROOT}/.assh/templates"

usage() {
  cat <<'EOF'
Usage:
  setup-production-ceph-storage.sh --config <path> --phase <phase> [--output-dir <path>] [--yes] [--execute]

Phases:
  inventory
  render
  plan
  apply-ceph-bootstrap
  apply-opennebula-datastores
  postcheck

Flags:
  --config <path>      Bash config file to source
  --phase <phase>      Phase to execute
  --output-dir <path>  Directory for rendered plans, logs, and state
  --yes                Auto-confirm readonly prompts only
  --execute            Required for mutating phases
  --help               Show this help

Notes:
  - Mutating phases always require interactive confirmation, even with --execute.
  - This script is additive only. It aborts rather than updating or removing existing
    production datastores, templates, or current local-root VM configuration.
EOF
}

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

fail() {
  printf 'ERROR: %s\n' "$*" >&2
  exit 1
}

require_binary() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required binary: $1"
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

join_by() {
  local delimiter="$1"
  shift || true
  local result=""
  local value
  for value in "$@"; do
    if [[ -z "${result}" ]]; then
      result="${value}"
    else
      result="${result}${delimiter}${value}"
    fi
  done
  printf '%s' "$result"
}

confirm_or_abort() {
  local prompt="$1"
  if [[ "${AUTO_YES}" == "true" && "${CURRENT_PHASE_MUTATES}" != "true" ]]; then
    log "auto-confirmed: ${prompt}"
    return 0
  fi

  local answer
  read -r -p "${prompt} [y/N]: " answer
  case "$(printf '%s' "${answer}" | tr '[:upper:]' '[:lower:]')" in
    y|yes)
      return 0
      ;;
    *)
      fail "aborted by operator"
      ;;
  esac
}

prepare_output_dirs() {
  mkdir -p "${OUTPUT_DIR}/logs" \
    "${OUTPUT_DIR}/rendered/datastores" \
    "${OUTPUT_DIR}/rendered/plans" \
    "${OUTPUT_DIR}/inventory/opennebula" \
    "${OUTPUT_DIR}/inventory/hosts" \
    "${OUTPUT_DIR}/state" \
    "${OUTPUT_DIR}/secrets"
  chmod 700 "${OUTPUT_DIR}/secrets" "${OUTPUT_DIR}/state"
}

run_logged_local() {
  local label="$1"
  shift
  log "local:${label}: $*"
  "$@" | tee "${OUTPUT_DIR}/logs/${label}.log"
}

run_local_capture() {
  local label="$1"
  shift
  log "capture:${label}: $*"
  "$@" >"${OUTPUT_DIR}/${label}"
}

run_remote_shell() {
  local host="$1"
  local command_string="$2"
  local label="$3"
  log "remote:${host}:${label}: ${command_string}"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "${command_string}")"
}

capture_remote_shell() {
  local host="$1"
  local command_string="$2"
  local output_file="$3"
  local label="$4"
  log "capture:${host}:${label}: ${command_string}"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "bash -lc $(printf '%q' "${command_string}")" >"${output_file}"
}

run_opennebula_shell() {
  local command_string="$1"
  local label="$2"
  if [[ "${OPENNEBULA_USE_SSH}" == "true" ]]; then
    run_remote_shell "${OPENNEBULA_FRONTEND_HOST}" "${command_string}" "${label}"
  else
    log "opennebula:${label}: ${command_string}"
    bash -lc "${command_string}"
  fi
}

capture_opennebula_shell() {
  local command_string="$1"
  local output_file="$2"
  local label="$3"
  if [[ "${OPENNEBULA_USE_SSH}" == "true" ]]; then
    capture_remote_shell "${OPENNEBULA_FRONTEND_HOST}" "${command_string}" "${output_file}" "${label}"
  else
    log "capture:opennebula:${label}: ${command_string}"
    bash -lc "${command_string}" >"${output_file}"
  fi
}

ceph_shell() {
  local command_string="$1"
  local label="$2"
  run_remote_shell "${CEPH_BOOTSTRAP_HOST}" "cephadm shell -- bash -lc $(printf '%q' "${command_string}")" "${label}"
}

capture_ceph_shell() {
  local command_string="$1"
  local output_file="$2"
  local label="$3"
  capture_remote_shell "${CEPH_BOOTSTRAP_HOST}" "cephadm shell -- bash -lc $(printf '%q' "${command_string}")" "${output_file}" "${label}"
}

require_phase_prereqs() {
  require_binary bash
  require_binary ssh
  [[ -n "${CONFIG_PATH}" ]] || fail "--config is required"
  [[ -f "${CONFIG_PATH}" ]] || fail "config file not found: ${CONFIG_PATH}"
  [[ -n "${PHASE}" ]] || fail "--phase is required"
}

ensure_mutation_allowed() {
  if [[ "${CURRENT_PHASE_MUTATES}" == "true" && "${EXECUTE}" != "true" ]]; then
    fail "phase ${PHASE} is mutating; rerun with --execute"
  fi
  if [[ "${CURRENT_PHASE_MUTATES}" == "true" && "${AUTO_YES}" == "true" ]]; then
    fail "--yes is only allowed for readonly phases"
  fi
}

ensure_required_config() {
  local required=(
    CEPH_BOOTSTRAP_HOST
    CEPH_BOOTSTRAP_MON_IP
    CEPH_PUBLIC_NETWORK
    CEPH_CLUSTER_NETWORK
    CEPH_POOL_SIZE
    CEPH_POOL_MIN_SIZE
    OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME
    OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME
    OPENNEBULA_CSI_RBD_DATASTORE_NAME
    OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME
    CEPH_RBD_POOL_IMAGES
    CEPH_RBD_POOL_SYSTEM
    CEPH_RBD_POOL_CSI
    CEPHFS_METADATA_POOL
    CEPHFS_DATA_POOL
    CEPHFS_NAME
    CEPHFS_ROOT_PATH
    CEPHFS_SUBVOLUME_GROUP
    CEPH_OPENNEBULA_CLIENT_ID
    CEPH_CSI_ADMIN_CLIENT_ID
    CEPH_CSI_NODE_CLIENT_ID
  )
  local name
  for name in "${required[@]}"; do
    [[ -n "${!name:-}" ]] || fail "config variable ${name} must be set"
  done
  [[ "${#CEPH_HOSTS[@]}" -gt 0 ]] || fail "CEPH_HOSTS must contain at least one host"
  [[ "${#INVENTORY_HOSTS[@]}" -gt 0 ]] || fail "INVENTORY_HOSTS must contain at least one host"
  [[ "${#OPENNEBULA_CEPH_CLIENT_HOSTS[@]}" -gt 0 ]] || fail "OPENNEBULA_CEPH_CLIENT_HOSTS must contain at least one host"
  [[ "${#HOST_PRIVATE_IP_MAP[@]}" -gt 0 ]] || fail "HOST_PRIVATE_IP_MAP must contain at least one host=ip entry"
  [[ "${#HOST_OSD_DEVICE_MAP[@]}" -gt 0 ]] || fail "HOST_OSD_DEVICE_MAP must contain at least one host=device entry"
}

load_config() {
  # shellcheck source=/dev/null
  source "${CONFIG_PATH}"

  : "${SSH_USER:=root}"
  : "${OPENNEBULA_USE_SSH:=false}"
  : "${OPENNEBULA_FRONTEND_HOST:=}"
  : "${OPENNEBULA_TALOS_TEMPLATE_NAME:=talos-omni-base}"
  : "${OPENNEBULA_CEPH_IMAGE_DATASTORE_TYPE:=IMAGE_DS}"
  : "${OPENNEBULA_CEPH_SYSTEM_DATASTORE_TYPE:=SYSTEM_DS}"
  : "${OPENNEBULA_CSI_RBD_DATASTORE_TYPE:=IMAGE_DS}"
  : "${OPENNEBULA_CSI_CEPHFS_DATASTORE_TYPE:=FILE_DS}"
  : "${CEPH_IMAGE:=}"
  : "${CEPH_FSID:=}"
  : "${CEPH_ALLOW_FQDN_HOSTNAMES:=true}"
  : "${CEPH_USE_CEPH_CONF:=false}"
  : "${CEPH_CONF_PATH:=/etc/ceph/ceph.conf}"
  : "${CEPH_LIBVIRT_SECRET_UUID:=}"
  : "${CEPH_LIBVIRT_SECRET_NAME:=client.opennebula secret}"
  : "${OPENNEBULA_CEPH_BRIDGE_LIST:=}"
  : "${OPENNEBULA_STAGING_DIR:=/var/tmp/opennebula-ceph-ops}"
  : "${OPENNEBULA_RBD_FORMAT:=2}"
  : "${OPENNEBULA_CEPH_KEY_PATH:=}"
  : "${OPENNEBULA_EXPECT_LOCAL_ROOT_SCHED_DS:=ID = 0}"
  : "${CEPH_MONITOR_PORT:=6789}"
  : "${CEPHFS_MOUNT_OPTIONS:=}"
  : "${CEPH_MGR_PLACEMENT_COUNT:=2}"
  : "${CEPH_MDS_PLACEMENT_COUNT:=2}"
  : "${OPENNEBULA_HOST_IDS:=}"
  : "${OPENNEBULA_DATASTORE_IDS:=}"
  : "${OPENNEBULA_TEMPLATE_NAMES:=}"

  ensure_required_config
}

map_get() {
  local array_name="$1"
  local lookup_key="$2"
  local entry key value
  eval "set -- \"\${${array_name}[@]}\""
  for entry in "$@"; do
    key="${entry%%=*}"
    value="${entry#*=}"
    if [[ "${key}" == "${lookup_key}" ]]; then
      printf '%s' "${value}"
      return 0
    fi
  done
  return 1
}

private_ip_for_host() {
  local host="$1"
  map_get HOST_PRIVATE_IP_MAP "${host}" || fail "HOST_PRIVATE_IP_MAP missing entry for ${host}"
}

osd_devices_for_host() {
  local host="$1"
  map_get HOST_OSD_DEVICE_MAP "${host}" || fail "HOST_OSD_DEVICE_MAP missing entry for ${host}"
}

phase_mutates() {
  case "${PHASE}" in
    apply-ceph-bootstrap|apply-opennebula-datastores)
      printf 'true'
      ;;
    *)
      printf 'false'
      ;;
  esac
}

lookup_datastore_id_by_name() {
  local datastore_name="$1"
  local xml
  xml="$(capture_opennebula_output "onedatastore list -x")"
  awk -v target="${datastore_name}" '
    BEGIN { RS="</DATASTORE>"; FS="\n" }
    $0 ~ "<NAME>" target "</NAME>" {
      if (match($0, /<ID>([0-9]+)<\/ID>/, m)) {
        print m[1]
        exit
      }
    }
  ' <<<"${xml}"
}

capture_opennebula_output() {
  local command_string="$1"
  if [[ "${OPENNEBULA_USE_SSH}" == "true" ]]; then
    ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${OPENNEBULA_FRONTEND_HOST}" "bash -lc $(printf '%q' "${command_string}")"
  else
    bash -lc "${command_string}"
  fi
}

capture_ceph_output() {
  local command_string="$1"
  ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${CEPH_BOOTSTRAP_HOST}" "bash -lc $(printf '%q' "cephadm shell -- bash -lc $(printf '%q' "${command_string}")")"
}

render_template_file() {
  local template_file="$1"
  local output_file="$2"
  shift 2

  local content
  content="$(<"${template_file}")"
  local pair
  for pair in "$@"; do
    local key="${pair%%=*}"
    local value="${pair#*=}"
    content="${content//__${key}__/${value}}"
  done
  printf '%s\n' "${content}" >"${output_file}"
}

normalize_bridge_list() {
  if [[ -n "${OPENNEBULA_CEPH_BRIDGE_LIST}" ]]; then
    printf '%s' "${OPENNEBULA_CEPH_BRIDGE_LIST}"
    return 0
  fi
  join_by " " "${OPENNEBULA_CEPH_CLIENT_HOSTS[@]}"
}

client_monitor_list() {
  if [[ -n "${CEPH_CLIENT_MONITORS:-}" ]]; then
    printf '%s' "${CEPH_CLIENT_MONITORS}"
    return 0
  fi
  local hosts=()
  local host
  for host in "${CEPH_HOSTS[@]}"; do
    hosts+=("$(private_ip_for_host "${host}")")
  done
  join_by " " "${hosts[@]}"
}

ceph_exists() {
  if capture_ceph_output "ceph fsid" >/dev/null 2>&1; then
    return 0
  fi
  return 1
}

datastore_exists() {
  [[ -n "$(lookup_datastore_id_by_name "$1")" ]]
}

write_plan_files() {
  local plan_file="${OUTPUT_DIR}/rendered/plans/ceph-bootstrap-plan.sh"
  local monitors
  monitors="$(client_monitor_list)"
  local bridge_list
  bridge_list="$(normalize_bridge_list)"

  cat >"${plan_file}" <<EOF
#!/usr/bin/env bash
set -euo pipefail

# Rendered by setup-production-ceph-storage.sh
# This file is a review artifact. Prefer using the main script for execution.

# Bootstrap host
#   ${CEPH_BOOTSTRAP_HOST}
# Ceph hosts
#   $(join_by ", " "${CEPH_HOSTS[@]}")
# Client monitor endpoints
#   ${monitors}
# OpenNebula bridge list
#   ${bridge_list}

# Bootstrap or validate Ceph cluster
sudo cephadm bootstrap --mon-ip ${CEPH_BOOTSTRAP_MON_IP} --cluster-network ${CEPH_CLUSTER_NETWORK}$(if [[ -n "${CEPH_IMAGE}" ]]; then printf ' --image %s' "${CEPH_IMAGE}"; fi)$(if [[ -n "${CEPH_FSID}" ]]; then printf ' --fsid %s' "${CEPH_FSID}"; fi)$(if [[ "${CEPH_ALLOW_FQDN_HOSTNAMES}" == "true" ]]; then printf ' --allow-fqdn-hostname'; fi)

# Add hosts and OSD devices
$(for host in "${CEPH_HOSTS[@]}"; do
  printf 'ceph orch host add %s %s\n' "${host}" "$(private_ip_for_host "${host}")"
  for device in $(osd_devices_for_host "${host}"); do
    printf 'ceph orch daemon add osd %s:%s\n' "${host}" "${device}"
  done
done)

# Pools
ceph osd pool create ${CEPH_RBD_POOL_IMAGES}
ceph osd pool create ${CEPH_RBD_POOL_SYSTEM}
ceph osd pool create ${CEPH_RBD_POOL_CSI}
ceph osd pool create ${CEPHFS_METADATA_POOL}
ceph osd pool create ${CEPHFS_DATA_POOL}

# CephFS
ceph fs new ${CEPHFS_NAME} ${CEPHFS_METADATA_POOL} ${CEPHFS_DATA_POOL}
ceph fs subvolumegroup create ${CEPHFS_NAME} ${CEPHFS_SUBVOLUME_GROUP}
EOF
  chmod 700 "${plan_file}"

  cat >"${OUTPUT_DIR}/rendered/plans/opennebula-datastores-plan.txt" <<EOF
Rendered datastore names:
  - ${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}
  - ${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}
  - ${OPENNEBULA_CSI_RBD_DATASTORE_NAME}
  - ${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}

Datastore contracts:
  ${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}: SYSTEM_DS, TM_MAD=ceph, DISK_TYPE=RBD, POOL_NAME=${CEPH_RBD_POOL_SYSTEM}
  ${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}: IMAGE_DS, DS_MAD=ceph, TM_MAD=ceph, DISK_TYPE=RBD, POOL_NAME=${CEPH_RBD_POOL_IMAGES}, COMPATIBLE_SYS_DS=<resolved at create time>
  ${OPENNEBULA_CSI_RBD_DATASTORE_NAME}: IMAGE_DS, DS_MAD=ceph, TM_MAD=ceph, DISK_TYPE=RBD, POOL_NAME=${CEPH_RBD_POOL_CSI}
  ${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}: FILE_DS, backend=cephfs, fs=${CEPHFS_NAME}, root=${CEPHFS_ROOT_PATH}, group=${CEPHFS_SUBVOLUME_GROUP}

Client monitor list:
  ${monitors}

Bridge list:
  ${bridge_list}
EOF
}

render_datastore_templates() {
  local monitors
  monitors="$(client_monitor_list)"
  local bridge_list
  bridge_list="$(normalize_bridge_list)"
  local secret_uuid="${CEPH_LIBVIRT_SECRET_UUID:-__AUTO_GENERATE__}"
  local system_ds_id="${OPENNEBULA_CEPH_SYSTEM_DATASTORE_FIXED_ID:-__AUTO_RESOLVE__}"
  local ceph_conf_value=""
  local ceph_key_value=""
  local ceph_conf_line=""
  local ceph_key_line=""
  local cephfs_mount_line=""

  if [[ "${CEPH_USE_CEPH_CONF}" == "true" ]]; then
    ceph_conf_line="CEPH_CONF = \"${CEPH_CONF_PATH}\""
    if [[ -n "${OPENNEBULA_CEPH_KEY_PATH}" ]]; then
      ceph_key_line="CEPH_KEY = \"${OPENNEBULA_CEPH_KEY_PATH}\""
    fi
  fi
  if [[ -n "${CEPHFS_MOUNT_OPTIONS}" ]]; then
    cephfs_mount_line="SPARKAI_CSI_CEPHFS_MOUNT_OPTIONS = \"${CEPHFS_MOUNT_OPTIONS}\""
  fi

  render_template_file \
    "${TEMPLATE_DIR}/datastore-ceph-system.tmpl" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}.tmpl" \
    "DATASTORE_NAME=${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}" \
    "CEPH_POOL_NAME=${CEPH_RBD_POOL_SYSTEM}" \
    "CEPH_HOST=${monitors}" \
    "CEPH_USER=${CEPH_OPENNEBULA_CLIENT_ID}" \
    "CEPH_SECRET=${secret_uuid}" \
    "BRIDGE_LIST=${bridge_list}" \
    "CEPH_CONF_LINE=${ceph_conf_line}" \
    "CEPH_KEY_LINE=${ceph_key_line}" \
    "RBD_FORMAT=${OPENNEBULA_RBD_FORMAT}"

  render_template_file \
    "${TEMPLATE_DIR}/datastore-ceph-images.tmpl" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}.tmpl" \
    "DATASTORE_NAME=${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}" \
    "CEPH_POOL_NAME=${CEPH_RBD_POOL_IMAGES}" \
    "CEPH_HOST=${monitors}" \
    "CEPH_USER=${CEPH_OPENNEBULA_CLIENT_ID}" \
    "CEPH_SECRET=${secret_uuid}" \
    "BRIDGE_LIST=${bridge_list}" \
    "COMPATIBLE_SYS_DS=${system_ds_id}" \
    "CEPH_CONF_LINE=${ceph_conf_line}" \
    "CEPH_KEY_LINE=${ceph_key_line}" \
    "RBD_FORMAT=${OPENNEBULA_RBD_FORMAT}" \
    "STAGING_DIR=${OPENNEBULA_STAGING_DIR}"

  render_template_file \
    "${TEMPLATE_DIR}/datastore-one-csi.tmpl" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CSI_RBD_DATASTORE_NAME}.tmpl" \
    "DATASTORE_NAME=${OPENNEBULA_CSI_RBD_DATASTORE_NAME}" \
    "CEPH_POOL_NAME=${CEPH_RBD_POOL_CSI}" \
    "CEPH_HOST=${monitors}" \
    "CEPH_USER=${CEPH_OPENNEBULA_CLIENT_ID}" \
    "CEPH_SECRET=${secret_uuid}" \
    "BRIDGE_LIST=${bridge_list}" \
    "CEPH_CONF_LINE=${ceph_conf_line}" \
    "CEPH_KEY_LINE=${ceph_key_line}" \
    "RBD_FORMAT=${OPENNEBULA_RBD_FORMAT}" \
    "STAGING_DIR=${OPENNEBULA_STAGING_DIR}"

  render_template_file \
    "${TEMPLATE_DIR}/datastore-one-csi-cephfs.tmpl" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}.tmpl" \
    "DATASTORE_NAME=${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}" \
    "CEPH_HOST=${monitors}" \
    "CEPHFS_NAME=${CEPHFS_NAME}" \
    "CEPHFS_ROOT_PATH=${CEPHFS_ROOT_PATH}" \
    "CEPHFS_SUBVOLUME_GROUP=${CEPHFS_SUBVOLUME_GROUP}" \
    "CEPHFS_MOUNT_OPTIONS_LINE=${cephfs_mount_line}" \
    "CEPH_CONF_LINE=${ceph_conf_line}"

  write_plan_files
}

inventory_phase() {
  log "running readonly inventory phase"

  capture_opennebula_shell "onehost list" "${OUTPUT_DIR}/inventory/opennebula/onehost-list.txt" "onehost-list"
  capture_opennebula_shell "onedatastore list" "${OUTPUT_DIR}/inventory/opennebula/onedatastore-list.txt" "onedatastore-list"
  capture_opennebula_shell "onetemplate list" "${OUTPUT_DIR}/inventory/opennebula/onetemplate-list.txt" "onetemplate-list"
  capture_opennebula_shell "onevnet list" "${OUTPUT_DIR}/inventory/opennebula/onevnet-list.txt" "onevnet-list"
  capture_opennebula_shell "onedatastore list -x" "${OUTPUT_DIR}/inventory/opennebula/onedatastore-list.xml" "onedatastore-list-xml"

  if [[ -n "${OPENNEBULA_TALOS_TEMPLATE_NAME}" ]]; then
    capture_opennebula_shell "onetemplate show ${OPENNEBULA_TALOS_TEMPLATE_NAME}" "${OUTPUT_DIR}/inventory/opennebula/talos-template.txt" "talos-template"
  fi

  local host_id
  for host_id in ${OPENNEBULA_HOST_IDS}; do
    capture_opennebula_shell "onehost show ${host_id}" "${OUTPUT_DIR}/inventory/opennebula/onehost-${host_id}.txt" "onehost-show-${host_id}"
  done

  local datastore_id
  for datastore_id in ${OPENNEBULA_DATASTORE_IDS}; do
    capture_opennebula_shell "onedatastore show ${datastore_id}" "${OUTPUT_DIR}/inventory/opennebula/onedatastore-${datastore_id}.txt" "onedatastore-show-${datastore_id}"
  done

  local template_name
  for template_name in ${OPENNEBULA_TEMPLATE_NAMES}; do
    capture_opennebula_shell "onetemplate show ${template_name}" "${OUTPUT_DIR}/inventory/opennebula/onetemplate-$(printf '%s' "${template_name}" | tr ' /' '__').txt" "onetemplate-show"
  done

  local host
  for host in "${INVENTORY_HOSTS[@]}"; do
    local host_dir="${OUTPUT_DIR}/inventory/hosts/${host}"
    mkdir -p "${host_dir}"
    capture_remote_shell "${host}" "hostname -f || hostname" "${host_dir}/hostname.txt" "hostname"
    capture_remote_shell "${host}" "ip -br addr" "${host_dir}/ip-br-addr.txt" "ip-br-addr"
    capture_remote_shell "${host}" "ip route" "${host_dir}/ip-route.txt" "ip-route"
    capture_remote_shell "${host}" "ip link" "${host_dir}/ip-link.txt" "ip-link"
    capture_remote_shell "${host}" "lsblk -e7 -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT,MODEL,SERIAL" "${host_dir}/lsblk.txt" "lsblk"
    capture_remote_shell "${host}" "blkid" "${host_dir}/blkid.txt" "blkid"
    capture_remote_shell "${host}" "pvs || true" "${host_dir}/pvs.txt" "pvs"
    capture_remote_shell "${host}" "vgs || true" "${host_dir}/vgs.txt" "vgs"
    capture_remote_shell "${host}" "lvs || true" "${host_dir}/lvs.txt" "lvs"
    capture_remote_shell "${host}" "systemctl list-units 'ceph*' 'opennebula*' --all" "${host_dir}/systemd-units.txt" "systemd-units"
    capture_remote_shell "${host}" "cephadm version || true" "${host_dir}/cephadm-version.txt" "cephadm-version"
    capture_remote_shell "${host}" "ceph --version || true" "${host_dir}/ceph-version.txt" "ceph-version"
    capture_remote_shell "${host}" "ss -lntup" "${host_dir}/ss-lntup.txt" "ss-lntup"
    capture_remote_shell "${host}" "if command -v nft >/dev/null 2>&1; then nft list ruleset; else iptables-save; fi" "${host_dir}/firewall.txt" "firewall"
  done

  log "inventory written to ${OUTPUT_DIR}/inventory"
}

render_phase() {
  log "rendering production Ceph storage artifacts"
  render_datastore_templates
  log "rendered plans and datastore templates written to ${OUTPUT_DIR}/rendered"
}

plan_phase() {
  render_datastore_templates
  cat <<EOF
Plan summary
============

Ceph bootstrap host:
  ${CEPH_BOOTSTRAP_HOST}

Ceph hosts:
  $(join_by ", " "${CEPH_HOSTS[@]}")

OpenNebula client hosts:
  $(join_by ", " "${OPENNEBULA_CEPH_CLIENT_HOSTS[@]}")

RBD pools:
  ${CEPH_RBD_POOL_IMAGES}
  ${CEPH_RBD_POOL_SYSTEM}
  ${CEPH_RBD_POOL_CSI}

CephFS:
  fs: ${CEPHFS_NAME}
  metadata pool: ${CEPHFS_METADATA_POOL}
  data pool: ${CEPHFS_DATA_POOL}
  subvolume group: ${CEPHFS_SUBVOLUME_GROUP}
  root path: ${CEPHFS_ROOT_PATH}

OpenNebula datastores:
  ${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}
  ${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}
  ${OPENNEBULA_CSI_RBD_DATASTORE_NAME}
  ${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}

Rendered artifacts:
  ${OUTPUT_DIR}/rendered/plans/ceph-bootstrap-plan.sh
  ${OUTPUT_DIR}/rendered/plans/opennebula-datastores-plan.txt
  ${OUTPUT_DIR}/rendered/datastores/
EOF
}

ensure_local_root_policy_still_holds() {
  if [[ -z "${OPENNEBULA_TALOS_TEMPLATE_NAME}" ]]; then
    return 0
  fi
  local template
  template="$(capture_opennebula_output "onetemplate show ${OPENNEBULA_TALOS_TEMPLATE_NAME}")"
  if ! grep -Fq "${OPENNEBULA_EXPECT_LOCAL_ROOT_SCHED_DS}" <<<"${template}"; then
    fail "template ${OPENNEBULA_TALOS_TEMPLATE_NAME} no longer contains expected local-root scheduling marker: ${OPENNEBULA_EXPECT_LOCAL_ROOT_SCHED_DS}"
  fi
}

ceph_pool_exists() {
  capture_ceph_output "ceph osd pool ls --format plain" | tr ' ' '\n' | grep -Fxq "$1"
}

ensure_ceph_pool() {
  local pool_name="$1"
  local app_name="$2"
  if ceph_pool_exists "${pool_name}"; then
    log "pool exists: ${pool_name}"
    return 0
  fi
  confirm_or_abort "Create Ceph pool ${pool_name}?"
  ceph_shell "ceph osd pool create ${pool_name}" "create-pool-${pool_name}"
  ceph_shell "ceph osd pool set ${pool_name} size ${CEPH_POOL_SIZE}" "pool-size-${pool_name}"
  ceph_shell "ceph osd pool set ${pool_name} min_size ${CEPH_POOL_MIN_SIZE}" "pool-min-size-${pool_name}"
  ceph_shell "ceph osd pool application enable ${pool_name} ${app_name}" "pool-app-${pool_name}"
  if [[ "${app_name}" == "rbd" ]]; then
    ceph_shell "rbd pool init -p ${pool_name}" "rbd-pool-init-${pool_name}"
  fi
}

ensure_cephfs() {
  if capture_ceph_output "ceph fs ls --format plain" | grep -Fq "${CEPHFS_NAME}"; then
    log "CephFS exists: ${CEPHFS_NAME}"
  else
    confirm_or_abort "Create CephFS ${CEPHFS_NAME}?"
    ceph_shell "ceph fs new ${CEPHFS_NAME} ${CEPHFS_METADATA_POOL} ${CEPHFS_DATA_POOL}" "cephfs-create"
  fi

  if capture_ceph_output "ceph fs subvolumegroup ls ${CEPHFS_NAME} --format json" | grep -Fq "\"name\": \"${CEPHFS_SUBVOLUME_GROUP}\""; then
    log "CephFS subvolume group exists: ${CEPHFS_SUBVOLUME_GROUP}"
  else
    confirm_or_abort "Create CephFS subvolume group ${CEPHFS_SUBVOLUME_GROUP}?"
    ceph_shell "ceph fs subvolumegroup create ${CEPHFS_NAME} ${CEPHFS_SUBVOLUME_GROUP}" "cephfs-subvol-group"
  fi

  local placement_hosts
  placement_hosts="$(join_by "," "${CEPH_HOSTS[@]}")"
  ceph_shell "ceph orch apply mds ${CEPHFS_NAME} --placement='${CEPH_MDS_PLACEMENT_COUNT} ${placement_hosts}'" "cephfs-mds-placement"
}

ensure_ceph_auth() {
  local opennebula_osd_caps
  opennebula_osd_caps="profile rbd pool=${CEPH_RBD_POOL_IMAGES}, profile rbd pool=${CEPH_RBD_POOL_SYSTEM}, profile rbd pool=${CEPH_RBD_POOL_CSI}"
  confirm_or_abort "Create or update Ceph auth user client.${CEPH_OPENNEBULA_CLIENT_ID} for OpenNebula RBD access?"
  ceph_shell "ceph auth get-or-create client.${CEPH_OPENNEBULA_CLIENT_ID} mon 'profile rbd' osd '${opennebula_osd_caps}'" "ceph-auth-opennebula"
  capture_ceph_shell "ceph auth get client.${CEPH_OPENNEBULA_CLIENT_ID}" "${OUTPUT_DIR}/secrets/ceph.client.${CEPH_OPENNEBULA_CLIENT_ID}.keyring" "ceph-auth-opennebula-keyring"
  capture_ceph_shell "ceph auth get-key client.${CEPH_OPENNEBULA_CLIENT_ID}" "${OUTPUT_DIR}/secrets/client.${CEPH_OPENNEBULA_CLIENT_ID}.key" "ceph-auth-opennebula-key"

  confirm_or_abort "Create or update Ceph auth user client.${CEPH_CSI_ADMIN_CLIENT_ID} for future CephFS controller use?"
  ceph_shell "ceph auth get-or-create client.${CEPH_CSI_ADMIN_CLIENT_ID} mon 'allow r' mds 'allow rw fsname=${CEPHFS_NAME} path=${CEPHFS_ROOT_PATH}' osd 'allow rw tag cephfs *=*' mgr 'allow rw'" "ceph-auth-csi-admin"
  capture_ceph_shell "ceph auth get client.${CEPH_CSI_ADMIN_CLIENT_ID}" "${OUTPUT_DIR}/secrets/ceph.client.${CEPH_CSI_ADMIN_CLIENT_ID}.keyring" "ceph-auth-csi-admin-keyring"
  capture_ceph_shell "ceph auth get-key client.${CEPH_CSI_ADMIN_CLIENT_ID}" "${OUTPUT_DIR}/secrets/client.${CEPH_CSI_ADMIN_CLIENT_ID}.key" "ceph-auth-csi-admin-key"

  confirm_or_abort "Create or update Ceph auth user client.${CEPH_CSI_NODE_CLIENT_ID} for future CephFS node-stage use?"
  ceph_shell "ceph auth get-or-create client.${CEPH_CSI_NODE_CLIENT_ID} mon 'allow r' mds 'allow rw fsname=${CEPHFS_NAME} path=${CEPHFS_ROOT_PATH}' osd 'allow rw tag cephfs *=*'" "ceph-auth-csi-node"
  capture_ceph_shell "ceph auth get client.${CEPH_CSI_NODE_CLIENT_ID}" "${OUTPUT_DIR}/secrets/ceph.client.${CEPH_CSI_NODE_CLIENT_ID}.keyring" "ceph-auth-csi-node-keyring"
  capture_ceph_shell "ceph auth get-key client.${CEPH_CSI_NODE_CLIENT_ID}" "${OUTPUT_DIR}/secrets/client.${CEPH_CSI_NODE_CLIENT_ID}.key" "ceph-auth-csi-node-key"
}

ensure_libvirt_secret_uuid() {
  if [[ -n "${CEPH_LIBVIRT_SECRET_UUID}" ]]; then
    printf '%s' "${CEPH_LIBVIRT_SECRET_UUID}" >"${OUTPUT_DIR}/state/ceph-libvirt-secret.uuid"
    return 0
  fi
  if [[ -f "${OUTPUT_DIR}/state/ceph-libvirt-secret.uuid" ]]; then
    CEPH_LIBVIRT_SECRET_UUID="$(<"${OUTPUT_DIR}/state/ceph-libvirt-secret.uuid")"
    export CEPH_LIBVIRT_SECRET_UUID
    return 0
  fi
  require_binary uuidgen
  CEPH_LIBVIRT_SECRET_UUID="$(uuidgen)"
  export CEPH_LIBVIRT_SECRET_UUID
  printf '%s' "${CEPH_LIBVIRT_SECRET_UUID}" >"${OUTPUT_DIR}/state/ceph-libvirt-secret.uuid"
}

prepare_opennebula_ceph_client_hosts() {
  ensure_libvirt_secret_uuid
  local raw_key
  raw_key="$(<"${OUTPUT_DIR}/secrets/client.${CEPH_OPENNEBULA_CLIENT_ID}.key")"

  local host
  for host in "${OPENNEBULA_CEPH_CLIENT_HOSTS[@]}"; do
    confirm_or_abort "Prepare Ceph client files and libvirt secret on OpenNebula host ${host}?"
    ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "mkdir -p /etc/ceph /var/lib/one/.ceph"
    ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "cat > /etc/ceph/ceph.client.${CEPH_OPENNEBULA_CLIENT_ID}.keyring" <"${OUTPUT_DIR}/secrets/ceph.client.${CEPH_OPENNEBULA_CLIENT_ID}.keyring"
    ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "cat > /var/lib/one/.ceph/client.${CEPH_OPENNEBULA_CLIENT_ID}.key" <<<"${raw_key}"
    ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "chmod 600 /etc/ceph/ceph.client.${CEPH_OPENNEBULA_CLIENT_ID}.keyring /var/lib/one/.ceph/client.${CEPH_OPENNEBULA_CLIENT_ID}.key"
    ssh -o StrictHostKeyChecking=accept-new "${SSH_USER}@${host}" "cat > /tmp/opennebula-ceph-secret.xml <<'EOF'
<secret ephemeral='no' private='no'>
  <uuid>${CEPH_LIBVIRT_SECRET_UUID}</uuid>
  <usage type='ceph'>
    <name>${CEPH_LIBVIRT_SECRET_NAME}</name>
  </usage>
</secret>
EOF
virsh -c qemu:///system secret-list --all | grep -Fq '${CEPH_LIBVIRT_SECRET_UUID}' || virsh -c qemu:///system secret-define /tmp/opennebula-ceph-secret.xml
virsh -c qemu:///system secret-set-value --secret '${CEPH_LIBVIRT_SECRET_UUID}' --base64 '${raw_key}'
rm -f /tmp/opennebula-ceph-secret.xml"
  done
}

apply_ceph_bootstrap_phase() {
  ensure_local_root_policy_still_holds
  confirm_or_abort "Proceed with Ceph bootstrap and host preparation on production?"

  if ceph_exists; then
    log "existing Ceph cluster detected on ${CEPH_BOOTSTRAP_HOST}"
    if [[ -n "${CEPH_FSID}" ]]; then
      local actual_fsid
      actual_fsid="$(capture_ceph_output "ceph fsid")"
      [[ "$(trim "${actual_fsid}")" == "${CEPH_FSID}" ]] || fail "existing Ceph FSID ${actual_fsid} does not match configured CEPH_FSID ${CEPH_FSID}"
    fi
  else
    confirm_or_abort "Bootstrap a new Ceph cluster on ${CEPH_BOOTSTRAP_HOST}?"
    local bootstrap_cmd
    bootstrap_cmd="cephadm bootstrap --mon-ip ${CEPH_BOOTSTRAP_MON_IP} --cluster-network ${CEPH_CLUSTER_NETWORK}"
    if [[ -n "${CEPH_IMAGE}" ]]; then
      bootstrap_cmd="${bootstrap_cmd} --image ${CEPH_IMAGE}"
    fi
    if [[ -n "${CEPH_FSID}" ]]; then
      bootstrap_cmd="${bootstrap_cmd} --fsid ${CEPH_FSID}"
    fi
    if [[ "${CEPH_ALLOW_FQDN_HOSTNAMES}" == "true" ]]; then
      bootstrap_cmd="${bootstrap_cmd} --allow-fqdn-hostname"
    fi
    run_remote_shell "${CEPH_BOOTSTRAP_HOST}" "${bootstrap_cmd}" "cephadm-bootstrap"
  fi

  ceph_shell "ceph config set mon public_network '${CEPH_PUBLIC_NETWORK}'" "ceph-config-public-network"
  ceph_shell "ceph config set global cluster_network '${CEPH_CLUSTER_NETWORK}'" "ceph-config-cluster-network"

  local placement_hosts
  placement_hosts="$(join_by "," "${CEPH_HOSTS[@]}")"
  local mgr_placement_hosts=("${CEPH_HOSTS[@]}")
  local mgr_placement
  mgr_placement="$(join_by "," "${mgr_placement_hosts[@]}")"

  local host
  for host in "${CEPH_HOSTS[@]}"; do
    confirm_or_abort "Ensure Ceph host ${host} is enrolled with orch?"
    ceph_shell "ceph orch host add ${host} $(private_ip_for_host "${host}")" "orch-host-add-${host}"
  done

  ceph_shell "ceph orch apply mon --placement='${placement_hosts}'" "orch-mon-placement"
  ceph_shell "ceph orch apply mgr --placement='${CEPH_MGR_PLACEMENT_COUNT} ${mgr_placement}'" "orch-mgr-placement"

  local device
  for host in "${CEPH_HOSTS[@]}"; do
    for device in $(osd_devices_for_host "${host}"); do
      confirm_or_abort "Add OSD ${device} on ${host}?"
      ceph_shell "ceph orch daemon add osd ${host}:${device}" "orch-osd-${host}-$(basename "${device}")"
    done
  done

  ensure_ceph_pool "${CEPH_RBD_POOL_IMAGES}" "rbd"
  ensure_ceph_pool "${CEPH_RBD_POOL_SYSTEM}" "rbd"
  ensure_ceph_pool "${CEPH_RBD_POOL_CSI}" "rbd"
  ensure_ceph_pool "${CEPHFS_METADATA_POOL}" "cephfs"
  ensure_ceph_pool "${CEPHFS_DATA_POOL}" "cephfs"

  ensure_cephfs
  ensure_ceph_auth
  prepare_opennebula_ceph_client_hosts

  render_datastore_templates
  log "Ceph bootstrap phase completed"
}

create_datastore_from_file() {
  local datastore_name="$1"
  local datastore_file="$2"
  if datastore_exists "${datastore_name}"; then
    log "datastore exists: ${datastore_name}"
    return 0
  fi
  confirm_or_abort "Create OpenNebula datastore ${datastore_name}?"
  run_opennebula_shell "onedatastore create ${datastore_file}" "onedatastore-create-$(printf '%s' "${datastore_name}" | tr ' /' '__')"
}

apply_opennebula_datastores_phase() {
  ensure_local_root_policy_still_holds
  ensure_libvirt_secret_uuid
  render_datastore_templates

  local ceph_system_id
  ceph_system_id="$(lookup_datastore_id_by_name "${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}")"
  if [[ -z "${ceph_system_id}" ]]; then
    create_datastore_from_file \
      "${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}" \
      "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}.tmpl"
    ceph_system_id="$(lookup_datastore_id_by_name "${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}")"
    [[ -n "${ceph_system_id}" ]] || fail "failed to resolve datastore ID after creating ${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}"
  fi

  render_template_file \
    "${TEMPLATE_DIR}/datastore-ceph-images.tmpl" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}.resolved.tmpl" \
    "DATASTORE_NAME=${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}" \
    "CEPH_POOL_NAME=${CEPH_RBD_POOL_IMAGES}" \
    "CEPH_HOST=$(client_monitor_list)" \
    "CEPH_USER=${CEPH_OPENNEBULA_CLIENT_ID}" \
    "CEPH_SECRET=${CEPH_LIBVIRT_SECRET_UUID}" \
    "BRIDGE_LIST=$(normalize_bridge_list)" \
    "COMPATIBLE_SYS_DS=${ceph_system_id}" \
    "CEPH_CONF_LINE=$(if [[ "${CEPH_USE_CEPH_CONF}" == "true" ]]; then printf 'CEPH_CONF = "%s"' "${CEPH_CONF_PATH}"; fi)" \
    "CEPH_KEY_LINE=$(if [[ "${CEPH_USE_CEPH_CONF}" == "true" && -n "${OPENNEBULA_CEPH_KEY_PATH}" ]]; then printf 'CEPH_KEY = "%s"' "${OPENNEBULA_CEPH_KEY_PATH}"; fi)" \
    "RBD_FORMAT=${OPENNEBULA_RBD_FORMAT}" \
    "STAGING_DIR=${OPENNEBULA_STAGING_DIR}"

  create_datastore_from_file \
    "${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}.resolved.tmpl"

  create_datastore_from_file \
    "${OPENNEBULA_CSI_RBD_DATASTORE_NAME}" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CSI_RBD_DATASTORE_NAME}.tmpl"

  create_datastore_from_file \
    "${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}" \
    "${OUTPUT_DIR}/rendered/datastores/${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}.tmpl"

  log "OpenNebula datastore phase completed"
}

postcheck_phase() {
  ensure_libvirt_secret_uuid
  capture_ceph_shell "ceph -s" "${OUTPUT_DIR}/inventory/postcheck-ceph-status.txt" "postcheck-ceph-status"
  capture_ceph_shell "ceph osd tree" "${OUTPUT_DIR}/inventory/postcheck-ceph-osd-tree.txt" "postcheck-osd-tree"
  capture_ceph_shell "ceph osd pool ls detail" "${OUTPUT_DIR}/inventory/postcheck-ceph-pools.txt" "postcheck-pools"
  capture_ceph_shell "ceph fs status" "${OUTPUT_DIR}/inventory/postcheck-cephfs-status.txt" "postcheck-cephfs-status"
  capture_ceph_shell "ceph fs subvolumegroup ls ${CEPHFS_NAME}" "${OUTPUT_DIR}/inventory/postcheck-cephfs-subvolumegroups.txt" "postcheck-cephfs-subvolumegroups"
  capture_ceph_shell "ceph auth ls" "${OUTPUT_DIR}/inventory/postcheck-ceph-auth.txt" "postcheck-ceph-auth"

  capture_opennebula_shell "onedatastore list" "${OUTPUT_DIR}/inventory/postcheck-onedatastore-list.txt" "postcheck-onedatastore-list"
  local datastore_name
  for datastore_name in \
    "${OPENNEBULA_CEPH_SYSTEM_DATASTORE_NAME}" \
    "${OPENNEBULA_CEPH_IMAGE_DATASTORE_NAME}" \
    "${OPENNEBULA_CSI_RBD_DATASTORE_NAME}" \
    "${OPENNEBULA_CSI_CEPHFS_DATASTORE_NAME}"; do
    local ds_id
    ds_id="$(lookup_datastore_id_by_name "${datastore_name}")"
    if [[ -n "${ds_id}" ]]; then
      capture_opennebula_shell "onedatastore show ${ds_id}" "${OUTPUT_DIR}/inventory/postcheck-$(printf '%s' "${datastore_name}" | tr ' /' '__').txt" "postcheck-${datastore_name}"
    fi
  done

  log "postcheck artifacts written to ${OUTPUT_DIR}/inventory"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --config)
        CONFIG_PATH="$2"
        shift 2
        ;;
      --phase)
        PHASE="$2"
        shift 2
        ;;
      --output-dir)
        OUTPUT_DIR="$2"
        shift 2
        ;;
      --yes)
        AUTO_YES="true"
        shift
        ;;
      --execute)
        EXECUTE="true"
        shift
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

CONFIG_PATH=""
PHASE=""
OUTPUT_DIR="${REPO_ROOT}/refs/tasks/production-ceph-storage"
AUTO_YES="false"
EXECUTE="false"
SSH_USER="root"

declare -a CEPH_HOSTS=()
declare -a INVENTORY_HOSTS=()
declare -a OPENNEBULA_CEPH_CLIENT_HOSTS=()
declare -a HOST_PRIVATE_IP_MAP=()
declare -a HOST_OSD_DEVICE_MAP=()

parse_args "$@"
require_phase_prereqs
load_config
prepare_output_dirs
CURRENT_PHASE_MUTATES="$(phase_mutates)"
ensure_mutation_allowed

case "${PHASE}" in
  inventory)
    inventory_phase
    ;;
  render)
    render_phase
    ;;
  plan)
    plan_phase
    ;;
  apply-ceph-bootstrap)
    apply_ceph_bootstrap_phase
    ;;
  apply-opennebula-datastores)
    apply_opennebula_datastores_phase
    ;;
  postcheck)
    postcheck_phase
    ;;
  *)
    fail "unsupported phase: ${PHASE}"
    ;;
esac
