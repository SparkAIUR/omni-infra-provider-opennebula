from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class DnsTlsPhase(BasePhase):
    name = "dns-tls"
    dependencies = ("omni",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        dns_script = ctx.repo_root / ".assh/scripts/bootstrap-dns.sh"
        frontend = ctx.config.nodes.frontend
        env = {
            "CF_API_TOKEN": ctx.config.dns.cloudflareToken,
            "CF_ZONE_NAME": ctx.config.dns.zone,
            "FRONTEND_IP": frontend.publicIP or frontend.hostname,
        }
        ctx.runner.run(["bash", str(dns_script)], env=env)
        nginx_path = ctx.renderer.render_to_path(
            "nginx-staging.conf.j2",
            ctx.rendered_dir / "dns-tls" / "nginx-staging.conf",
            config=ctx.config,
        )
        return {"dns_records": ctx.config.dns.records, "nginx_config": str(nginx_path)}

