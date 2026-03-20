from pathlib import Path

from labctl.config import load_config
from labctl.render import Renderer


def test_provider_rendering_contains_staging_defaults(tmp_path: Path) -> None:
    root = Path(__file__).resolve().parents[1]
    config = load_config(root / "examples" / "staging-lab.config.example.yaml")
    renderer = Renderer(config)
    output = tmp_path / "provider-config.yaml"
    renderer.render_to_path("provider-config.yaml.j2", output, config=config)
    body = output.read_text()
    assert 'templateName: "talos-omni-base"' in body
    assert 'defaultDatastore: "default"' in body
    assert "opennebula-{{ .Arch }}.qcow2" in body


def test_handoff_rendering_contains_expected_cluster_settings(tmp_path: Path) -> None:
    root = Path(__file__).resolve().parents[1]
    config = load_config(root / "examples" / "staging-lab.config.example.yaml")
    renderer = Renderer(config)
    output = tmp_path / "handoff.yaml"
    renderer.render_to_path("handoff-cluster.yaml.j2", output, config=config)
    body = output.read_text()
    assert "name: hplcsi" in body
    assert "version: v1.12.5" in body
    assert "version: v1.34.5" in body
