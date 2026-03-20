from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class OmniPhase(BasePhase):
    name = "omni"
    dependencies = ("dex",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        config_path = ctx.renderer.render_to_path(
            "omni-config.yaml.j2",
            ctx.rendered_dir / "omni" / "config.yaml",
            config=ctx.config,
        )
        admin_env = ctx.renderer.render_to_path(
            "omni-admin.env.j2",
            ctx.rendered_dir / "omni" / "admin.env",
            config=ctx.config,
        )
        return {"config": str(config_path), "admin_env": str(admin_env)}

