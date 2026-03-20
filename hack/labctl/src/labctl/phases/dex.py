from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class DexPhase(BasePhase):
    name = "dex"
    dependencies = ("opennebula",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        config_path = ctx.renderer.render_to_path(
            "dex-config.yaml.j2",
            ctx.rendered_dir / "dex" / "config.yaml",
            config=ctx.config,
        )
        return {"config": str(config_path), "issuer": ctx.config.dex.publicURL}

