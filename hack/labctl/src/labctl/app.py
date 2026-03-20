from __future__ import annotations

from dataclasses import dataclass

from .config import RootConfig
from .phases import (
    BasePhase,
    CephPhase,
    DexPhase,
    DnsTlsPhase,
    HandoffClusterPhase,
    HostBootstrapPhase,
    NetworkOverlayPhase,
    OmniPhase,
    OpenNebulaPhase,
    PhaseContext,
    ProviderPhase,
    SpotProvisionPhase,
    ValidationPhase,
)
from .remote import RemoteClient
from .render import Renderer
from .runner import Runner
from .state import StateStore

PHASES: tuple[type[BasePhase], ...] = (
    SpotProvisionPhase,
    HostBootstrapPhase,
    NetworkOverlayPhase,
    CephPhase,
    OpenNebulaPhase,
    DexPhase,
    OmniPhase,
    DnsTlsPhase,
    ProviderPhase,
    ValidationPhase,
    HandoffClusterPhase,
)


def _collect_secrets(config: RootConfig) -> list[str]:
    secrets = [
        config.opennebula.oneadminPassword,
        config.dex.password,
        config.dns.cloudflareToken,
        config.omni.serviceAccount.key,
        config.omni.oidc.clientSecret,
    ]
    secrets.extend(config.provider.env.values())
    secrets.extend(client.secret for client in config.dex.staticClients)
    return [value for value in secrets if value and "<secret" not in value]


@dataclass
class LabApp:
    config: RootConfig

    def __post_init__(self) -> None:
        self.config.state_dir.mkdir(parents=True, exist_ok=True)
        self.config.workspace_dir.mkdir(parents=True, exist_ok=True)
        self.config.log_dir.mkdir(parents=True, exist_ok=True)
        self.state = StateStore(self.config.state_path)
        self.runner = Runner(self.config.log_dir, secrets=_collect_secrets(self.config))
        self.remote = RemoteClient(self.config, self.runner)
        self.renderer = Renderer(self.config)
        self.ctx = PhaseContext(
            config=self.config,
            state=self.state,
            runner=self.runner,
            remote=self.remote,
            renderer=self.renderer,
        )
        self.phase_map = {phase_cls.name: phase_cls() for phase_cls in PHASES}

    def ordered_phase_names(self) -> list[str]:
        return [phase_cls.name for phase_cls in PHASES]

    def enabled_phase_names(self) -> list[str]:
        return [name for name, phase in self.phase_map.items() if phase.is_enabled(self.ctx)]

    def plan(self) -> list[dict[str, str]]:
        plan: list[dict[str, str]] = []
        for name in self.ordered_phase_names():
            phase = self.phase_map[name]
            plan.append(
                {
                    "name": name,
                    "enabled": "yes" if phase.is_enabled(self.ctx) else "no",
                    "status": self.state.phase(name).status,
                }
            )
        return plan

    def run_phase(self, name: str, *, force: bool = False) -> None:
        if name not in self.phase_map:
            raise KeyError(f"unknown phase: {name}")
        for dependency in self.phase_map[name].dependencies:
            self.run_phase(dependency, force=force)
        self.phase_map[name].execute(self.ctx, force=force)

    def run_all(self, *, force: bool = False) -> None:
        for name in self.ordered_phase_names():
            self.run_phase(name, force=force)

    def resume(self, *, force: bool = False) -> str | None:
        phase_name = self.state.first_incomplete(self.ordered_phase_names())
        if phase_name is None:
            return None
        self.run_phase(phase_name, force=force)
        remaining = self.ordered_phase_names()[self.ordered_phase_names().index(phase_name) + 1 :]
        for name in remaining:
            self.run_phase(name, force=force)
        return phase_name
