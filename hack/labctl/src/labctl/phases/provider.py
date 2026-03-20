from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class ProviderPhase(BasePhase):
    name = "provider"
    dependencies = ("dns-tls",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        provider_config_path = ctx.renderer.render_to_path(
            "provider-config.yaml.j2",
            ctx.rendered_dir / "provider" / "config.yaml",
            config=ctx.config,
        )
        env_path = ctx.renderer.render_to_path(
            "provider.env.j2",
            ctx.rendered_dir / "provider" / "provider.env",
            config=ctx.config,
        )
        script = ctx.repo_root / ".assh/scripts/bootstrap-provider.sh"
        env = {
            "TARGET": ctx.config.nodes.frontend.publicIP or ctx.config.nodes.frontend.hostname,
            "OMNI_ENDPOINT": ctx.config.omni.publicURL,
            "OMNI_SERVICE_ACCOUNT_KEY": ctx.config.provider.env["OMNI_SERVICE_ACCOUNT_KEY"],
            "OPENNEBULA_USERNAME": ctx.config.provider.env["OPENNEBULA_USERNAME"],
            "OPENNEBULA_PASSWORD": ctx.config.provider.env["OPENNEBULA_PASSWORD"],
            "PROVIDER_ROOT": ctx.config.provider.workingDir,
            "PROVIDER_IMAGE": ctx.config.provider.image,
            "PROVIDER_CONFIG_SOURCE": str(provider_config_path),
        }
        ctx.runner.run(["bash", str(script)], env=env)
        return {
            "provider_config": str(provider_config_path),
            "provider_env": str(env_path),
            "working_dir": ctx.config.provider.workingDir,
        }
