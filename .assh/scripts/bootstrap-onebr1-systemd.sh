#!/usr/bin/env bash
set -euo pipefail

: "${TARGET:?set TARGET}"
: "${LOCAL_IP:?set LOCAL_IP}"
: "${PEERS:?set PEERS comma-separated private underlay IPs}"

bridge="${BRIDGE_NAME:-onebr1}"
vxlan_dev="${VXLAN_DEV:-vxlan-onebr1}"
underlay_dev="${UNDERLAY_DEV:-enp3s0}"
vni="${VXLAN_VNI:-422}"
mtu="${BRIDGE_MTU:-1450}"
gateway_ip="${GATEWAY_IP:-}"
nat_cidr="${NAT_CIDR:-172.22.0.0/24}"

peer_lines=""
IFS=',' read -r -a peer_list <<<"${PEERS}"
for peer in "${peer_list[@]}"; do
  peer="${peer// /}"
  [[ -n "${peer}" ]] || continue
  peer_lines+="bridge fdb append 00:00:00:00:00:00 dev ${vxlan_dev} dst ${peer} 2>/dev/null || true"$'\n'
done

read -r -d '' remote_script <<EOF || true
#!/usr/bin/env bash
set -euo pipefail

bridge="${bridge}"
vxlan_dev="${vxlan_dev}"
underlay_dev="${underlay_dev}"
local_ip="${LOCAL_IP}"
vni="${vni}"
mtu="${mtu}"
gateway_ip="${gateway_ip}"
nat_cidr="${nat_cidr}"

modprobe vxlan

ip link show "\${bridge}" >/dev/null 2>&1 || ip link add "\${bridge}" type bridge
ip link set "\${bridge}" mtu "\${mtu}" || true
ip link set "\${bridge}" up

if ip link show "\${vxlan_dev}" >/dev/null 2>&1; then
  ip link set "\${vxlan_dev}" down || true
  ip link del "\${vxlan_dev}" || true
fi

ip link add "\${vxlan_dev}" type vxlan id "\${vni}" dstport 4789 nolearning dev "\${underlay_dev}" local "\${local_ip}"
ip link set "\${vxlan_dev}" mtu "\${mtu}"
ip link set "\${vxlan_dev}" master "\${bridge}"
ip link set "\${vxlan_dev}" up
bridge link set dev "\${vxlan_dev}" learning off flood on

${peer_lines}

if [[ -n "\${gateway_ip}" ]]; then
  ip addr show dev "\${bridge}" | grep -q "\${gateway_ip%%/*}" || ip addr add "\${gateway_ip}" dev "\${bridge}"
  sysctl -w net.ipv4.ip_forward=1 >/dev/null
  iptables -t nat -C POSTROUTING -s "\${nat_cidr}" -o "\${underlay_dev}" -j MASQUERADE 2>/dev/null || \
    iptables -t nat -A POSTROUTING -s "\${nat_cidr}" -o "\${underlay_dev}" -j MASQUERADE
  iptables -t mangle -C FORWARD -s "\${nat_cidr}" -o "\${underlay_dev}" -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || \
    iptables -t mangle -I FORWARD 1 -s "\${nat_cidr}" -o "\${underlay_dev}" -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
  iptables -t mangle -C FORWARD -d "\${nat_cidr}" -i "\${underlay_dev}" -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || \
    iptables -t mangle -I FORWARD 1 -d "\${nat_cidr}" -i "\${underlay_dev}" -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
fi
EOF

read -r -d '' remote_unit <<EOF || true
[Unit]
Description=Bring up ${bridge} VXLAN guest overlay
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/sbin/${bridge}-up.sh

[Install]
WantedBy=multi-user.target
EOF

tmp_script="$(mktemp)"
tmp_unit="$(mktemp)"
trap 'rm -f "${tmp_script}" "${tmp_unit}"' EXIT
printf '%s\n' "${remote_script}" >"${tmp_script}"
printf '%s\n' "${remote_unit}" >"${tmp_unit}"

scp -o StrictHostKeyChecking=accept-new "${tmp_script}" "root@${TARGET}:/usr/local/sbin/${bridge}-up.sh" >/dev/null
scp -o StrictHostKeyChecking=accept-new "${tmp_unit}" "root@${TARGET}:/etc/systemd/system/${bridge}.service" >/dev/null
ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "chmod 0755 /usr/local/sbin/${bridge}-up.sh && systemctl daemon-reload && systemctl enable --now ${bridge}.service && systemctl --no-pager --full status ${bridge}.service"
