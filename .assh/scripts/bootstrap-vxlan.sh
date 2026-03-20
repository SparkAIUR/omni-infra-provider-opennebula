#!/usr/bin/env bash
set -euo pipefail

: "${TARGET:?set TARGET}"
: "${VXLAN_PEERS:?set VXLAN_PEERS comma-separated}"

vni="${VXLAN_VNI:-4000}"
bridge="${VXLAN_BRIDGE:-br-talos-vxlan}"
dev="${VXLAN_DEVICE:-vxlan-talos}"
underlay="${UNDERLAY_DEVICE:-enX0}"
mtu="${VXLAN_MTU:-1450}"
gateway_ip="${VXLAN_GATEWAY_IP:-}"
nat_cidr="${VXLAN_CIDR:-10.77.0.0/24}"

peer_args=()
IFS=',' read -r -a peers <<<"${VXLAN_PEERS}"
for peer in "${peers[@]}"; do
  peer_args+=("bridge" "fdb" "append" "00:00:00:00:00:00" "dev" "${dev}" "dst" "${peer}")
done

ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "bash -s" <<EOF
set -euo pipefail
ip link show ${bridge} >/dev/null 2>&1 || ip link add ${bridge} type bridge
ip link set ${bridge} up
if ip link show ${dev} >/dev/null 2>&1; then
  if ! ip -d link show ${dev} | grep -q "nolearning"; then
    ip link set ${dev} down || true
    ip link del ${dev}
  fi
fi
ip link show ${dev} >/dev/null 2>&1 || ip link add ${dev} type vxlan id ${vni} dstport 4789 nolearning local \$(ip -4 -o addr show ${underlay} | awk '{print \$4}' | cut -d/ -f1)
ip link set ${dev} mtu ${mtu}
ip link set ${dev} master ${bridge}
ip link set ${dev} up
bridge link set dev ${dev} learning off flood on
$(for ((i=0; i<${#peer_args[@]}; i+=8)); do printf "bridge fdb show dev %s | grep -q 'dst %s' || bridge fdb append 00:00:00:00:00:00 dev %s dst %s\n" "${dev}" "${peer_args[i+7]}" "${dev}" "${peer_args[i+7]}"; done)
EOF

if [[ -n "${gateway_ip}" ]]; then
  ssh -o StrictHostKeyChecking=accept-new "root@${TARGET}" "bash -s" <<EOF
set -euo pipefail
ip addr show ${bridge} | grep -q '${gateway_ip}' || ip addr add ${gateway_ip} dev ${bridge}
sysctl -w net.ipv4.ip_forward=1 >/dev/null
iptables -t nat -C POSTROUTING -s ${nat_cidr} -o ${underlay} -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING -s ${nat_cidr} -o ${underlay} -j MASQUERADE
iptables -t mangle -C FORWARD -s ${nat_cidr} -o ${underlay} -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || iptables -t mangle -I FORWARD 1 -s ${nat_cidr} -o ${underlay} -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
iptables -t mangle -C FORWARD -d ${nat_cidr} -i ${underlay} -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu 2>/dev/null || iptables -t mangle -I FORWARD 1 -d ${nat_cidr} -i ${underlay} -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
EOF
fi
