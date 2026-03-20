from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext


class ValidationPhase(BasePhase):
    name = "validation"
    dependencies = ("provider",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        frontend = ctx.config.nodes.frontend.publicIP or ctx.config.nodes.frontend.hostname
        run_e2e = ctx.repo_root / ".assh/scripts/run-provider-e2e.sh"
        collect = ctx.repo_root / ".assh/scripts/collect-staging-artifacts.sh"
        ctx.runner.run(["bash", str(run_e2e)], env={"TARGET": frontend})
        artifact_result = ctx.runner.run(
            ["bash", str(collect)],
            env={"TARGET": frontend, "OUT_DIR": str(ctx.config.workspace_dir / "artifacts")},
        )
        artifact_dir = artifact_result.stdout.strip().splitlines()[-1]
        ctx.state.update_artifact("validation", artifact_dir)
        return {"frontend": frontend, "artifact_dir": artifact_dir}

