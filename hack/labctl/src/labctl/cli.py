from __future__ import annotations

import json
from pathlib import Path
from typing import Annotated

import typer

from .app import LabApp
from .config import load_config

app = typer.Typer(add_completion=False, no_args_is_help=True)
ConfigOption = Annotated[Path, typer.Option("--config", exists=True, readable=True)]
ForceOption = Annotated[bool, typer.Option("--force")]


def build_app(config: Path) -> LabApp:
    return LabApp(load_config(config))


@app.callback()
def callback(
    ctx: typer.Context,
    config: ConfigOption,
) -> None:
    """Manage the OpenNebula staging lab."""
    ctx.obj = {"config": config}


@app.command()
def plan(ctx: typer.Context) -> None:
    lab = build_app(ctx.obj["config"])
    typer.echo(json.dumps(lab.plan(), indent=2))


@app.command()
def run(ctx: typer.Context, force: ForceOption = False) -> None:
    build_app(ctx.obj["config"]).run_all(force=force)


@app.command("phase")
def phase_command(ctx: typer.Context, phase_name: str, force: ForceOption = False) -> None:
    build_app(ctx.obj["config"]).run_phase(phase_name, force=force)


@app.command()
def resume(ctx: typer.Context, force: ForceOption = False) -> None:
    resumed = build_app(ctx.obj["config"]).resume(force=force)
    if resumed is None:
        typer.echo("all phases already completed")
    else:
        typer.echo(f"resumed from {resumed}")


@app.command()
def status(ctx: typer.Context) -> None:
    lab = build_app(ctx.obj["config"])
    typer.echo(json.dumps(lab.state.data, indent=2))


@app.command()
def validate(ctx: typer.Context) -> None:
    build_app(ctx.obj["config"]).run_phase("validation", force=True)


@app.command("collect-artifacts")
def collect_artifacts(ctx: typer.Context) -> None:
    build_app(ctx.obj["config"]).run_phase("validation", force=True)


@app.command("destroy-handoff-cluster")
def destroy_handoff_cluster(ctx: typer.Context) -> None:
    lab = build_app(ctx.obj["config"])
    rendered = lab.config.rendered_dir / "handoff-cluster" / "destroy.txt"
    rendered.parent.mkdir(parents=True, exist_ok=True)
    host = lab.config.nodes.frontend.publicIP or lab.config.nodes.frontend.hostname
    env = lab.config.omni.adminEnvPath
    name = lab.config.handoffCluster.name
    rendered.write_text(
        f"ssh root@{host} \"set -a; . {env}; set +a; omnictl cluster delete {name}\"\n"
    )
    typer.echo(str(rendered))


@app.command("recreate-handoff-cluster")
def recreate_handoff_cluster(ctx: typer.Context, force: ForceOption = False) -> None:
    build_app(ctx.obj["config"]).run_phase("handoff-cluster", force=force)


def main() -> None:
    app()


if __name__ == "__main__":
    main()
