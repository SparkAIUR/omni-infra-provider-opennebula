from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

from ..config import RootConfig
from ..remote import RemoteClient
from ..render import Renderer
from ..runner import Runner
from ..state import PHASE_COMPLETED, PHASE_FAILED, PHASE_RUNNING, PHASE_SKIPPED, StateStore


@dataclass
class PhaseContext:
    config: RootConfig
    state: StateStore
    runner: Runner
    remote: RemoteClient
    renderer: Renderer

    @property
    def repo_root(self) -> Path:
        return self.config.repo_root

    @property
    def rendered_dir(self) -> Path:
        self.config.rendered_dir.mkdir(parents=True, exist_ok=True)
        return self.config.rendered_dir


class BasePhase:
    name = "base"
    dependencies: tuple[str, ...] = ()

    def is_enabled(self, ctx: PhaseContext) -> bool:
        return True

    def should_run(self, ctx: PhaseContext, *, force: bool) -> bool:
        if force:
            return True
        phase = ctx.state.phase(self.name)
        if phase.status in {PHASE_FAILED, PHASE_RUNNING}:
            return True
        return not (
            phase.status == PHASE_COMPLETED
            and phase.config_hash == ctx.config.config_hash()
        )

    def execute(self, ctx: PhaseContext, *, force: bool = False) -> None:
        if not self.is_enabled(ctx):
            ctx.state.update_phase(
                self.name,
                status=PHASE_SKIPPED,
                config_hash=ctx.config.config_hash(),
            )
            return
        if not self.should_run(ctx, force=force):
            return
        ctx.state.update_phase(
            self.name,
            status=PHASE_RUNNING,
            config_hash=ctx.config.config_hash(),
        )
        try:
            details = self.run(ctx) or {}
        except Exception as exc:
            details = {"error": str(exc)}
            ctx.state.update_phase(
                self.name,
                status=PHASE_FAILED,
                config_hash=ctx.config.config_hash(),
                details=details,
            )
            raise
        ctx.state.update_phase(
            self.name,
            status=PHASE_COMPLETED,
            config_hash=ctx.config.config_hash(),
            details=details,
        )

    def run(self, ctx: PhaseContext) -> dict[str, Any] | None:
        raise NotImplementedError
