from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class SpotProvisionPhase(BasePhase):
    name = "spot-provision"

    def is_enabled(self, ctx: PhaseContext) -> bool:
        return ctx.config.rackspace.enabled

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        rackspace = ctx.config.rackspace
        if not rackspace.rsspotConfigPath:
            raise ValueError("rackspace.rsspotConfigPath is required when rackspace.enabled=true")
        try:
            import rsspot  # type: ignore # noqa: F401
        except ImportError as exc:
            raise RuntimeError(
                "rackspace provisioning requested but rsspot is not importable in this environment"
            ) from exc

        discovered = {
            node.hostname: {
                "public_ip": node.publicIP,
                "private_ip": node.privateIP,
                "status": "configured",
            }
            for node in ctx.config.nodes.all_nodes()
        }
        ctx.state.update_fact("rackspace_nodes", discovered)
        return {
            "note": "rsspot integration hook validated; node reconciliation is operator-configured",
            "nodes": discovered,
        }

