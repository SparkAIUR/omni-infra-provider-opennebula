from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class CephPhase(BasePhase):
    name = "ceph"
    dependencies = ("network-overlay",)

    def is_enabled(self, ctx: PhaseContext) -> bool:
        return ctx.config.ceph.enabled

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        frontend = ctx.config.nodes.frontend
        rendered = ctx.renderer.render_to_path(
            "ceph-bootstrap.sh.j2",
            ctx.rendered_dir / "ceph" / "bootstrap.sh",
            config=ctx.config,
        )
        return {
            "frontend": frontend.hostname,
            "bootstrap_script": str(rendered),
            "mons": ctx.config.ceph.mons,
            "osd_device": ctx.config.ceph.rawOsdDevice,
        }

