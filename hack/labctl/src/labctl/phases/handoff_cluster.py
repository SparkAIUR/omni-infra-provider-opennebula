from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class HandoffClusterPhase(BasePhase):
    name = "handoff-cluster"
    dependencies = ("validation",)

    def is_enabled(self, ctx: PhaseContext) -> bool:
        return ctx.config.handoffCluster.enabled

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        machineclasses = ctx.renderer.render_to_path(
            "handoff-machineclasses.yaml.j2",
            ctx.rendered_dir / "handoff-cluster" / "machineclasses.yaml",
            config=ctx.config,
        )
        cluster = ctx.renderer.render_to_path(
            "handoff-cluster.yaml.j2",
            ctx.rendered_dir / "handoff-cluster" / "cluster.yaml",
            config=ctx.config,
        )
        return {"machineclasses": str(machineclasses), "cluster": str(cluster)}

