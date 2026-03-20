from __future__ import annotations

from typing import Any

from .base import BasePhase, PhaseContext

HOST_BOOTSTRAP_SCRIPT = r"""#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

hostnamectl set-hostname "{{ hostname }}"
apt-get update
apt-get install -y curl ca-certificates gnupg jq python3 docker.io qemu-utils bridge-utils
timedatectl set-ntp true

if mountpoint -q /mnt; then
  umount /mnt
fi
sed -i '\# /mnt #d' /etc/fstab

if [[ -b "{{ raw_osd_device }}" ]]; then
  wipefs -af "{{ raw_osd_device }}" || true
fi
"""


class HostBootstrapPhase(BasePhase):
    name = "host-bootstrap"
    dependencies = ("spot-provision",)

    def run(self, ctx: PhaseContext) -> dict[str, Any]:
        results: dict[str, Any] = {}
        script_template = ctx.renderer.env.from_string(HOST_BOOTSTRAP_SCRIPT)
        for node in ctx.config.nodes.all_nodes():
            rendered = script_template.render(
                hostname=node.hostname,
                raw_osd_device=ctx.config.ceph.rawOsdDevice,
            )
            path = ctx.rendered_dir / "host-bootstrap" / f"{node.hostname}.sh"
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(rendered + "\n")
            results[node.hostname] = {"script": str(path)}
        return results

