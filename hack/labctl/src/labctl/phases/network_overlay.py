from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class NetworkOverlayPhase(BasePhase):
    name = "network-overlay"
    dependencies = ("host-bootstrap",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        script = ctx.repo_root / ".assh/scripts/bootstrap-onebr1-systemd.sh"
        results: dict[str, Any] = {}
        peer_ips = [node.privateIP or "" for node in ctx.config.nodes.all_nodes() if node.privateIP]
        peers = ",".join(peer_ips)
        for node in ctx.config.nodes.all_nodes():
            env = {
                "TARGET": node.publicIP or node.hostname,
                "LOCAL_IP": node.privateIP or "",
                "PEERS": peers,
                "BRIDGE_NAME": ctx.config.networking.guestBridge,
                "VXLAN_DEV": ctx.config.networking.guestVXLANDevice,
                "UNDERLAY_DEV": ctx.config.networking.underlayInterface,
                "VXLAN_VNI": str(ctx.config.networking.vxlanVni),
                "BRIDGE_MTU": str(ctx.config.networking.guestMTU),
                "NAT_CIDR": ctx.config.networking.natCIDR,
            }
            if node.hostname == ctx.config.nodes.frontend.hostname:
                env["GATEWAY_IP"] = ctx.config.networking.guestGatewayCIDR
            ctx.runner.run(["bash", str(script)], env=env)
            results[node.hostname] = {
                "target": env["TARGET"],
                "bridge": ctx.config.networking.guestBridge,
            }
        return results
