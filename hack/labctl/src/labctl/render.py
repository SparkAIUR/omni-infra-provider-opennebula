from __future__ import annotations

from pathlib import Path

from jinja2 import Environment, FileSystemLoader

from .config import RootConfig


class Renderer:
    def __init__(self, config: RootConfig) -> None:
        template_root = Path(__file__).resolve().parents[2] / "templates"
        self.env = Environment(loader=FileSystemLoader(str(template_root)), autoescape=False)
        self.config = config

    def render_to_path(self, template_name: str, destination: Path, **context: object) -> Path:
        destination.parent.mkdir(parents=True, exist_ok=True)
        template = self.env.get_template(template_name)
        destination.write_text(template.render(**context) + "\n")
        return destination
