from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class OpenNebulaPhase(BasePhase):
    name = "opennebula"
    dependencies = ("ceph",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        datastores = [store.model_dump(mode="json") for store in ctx.config.opennebula.datastores]
        networks = [network.model_dump(mode="json") for network in ctx.config.opennebula.networks]
        rendered = {
            "template": ctx.renderer.render_to_path(
                "opennebula-template.txt.j2",
                ctx.rendered_dir / "opennebula" / "talos-omni-base.txt",
                config=ctx.config,
            ),
            "recovery": ctx.renderer.render_to_path(
                "fireedge-secret-recovery.sh.j2",
                ctx.rendered_dir / "opennebula" / "fireedge-secret-recovery.sh",
                config=ctx.config,
            ),
        }
        return {
            "template_name": ctx.config.opennebula.template.name,
            "datastores": datastores,
            "networks": networks,
            "rendered_files": {key: str(path) for key, path in rendered.items()},
        }

