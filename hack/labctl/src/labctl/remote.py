from __future__ import annotations

import time
from pathlib import Path

from .config import NodeConfig, RootConfig
from .runner import CommandError, Runner


class RemoteClient:
    def __init__(self, config: RootConfig, runner: Runner) -> None:
        self.config = config
        self.runner = runner

    def ssh_target(self, node: NodeConfig) -> str:
        host = node.publicIP or node.hostname
        return f"{node.sshUser}@{host}"

    def ssh_base_args(self) -> list[str]:
        ssh = self.config.ssh
        return [
            "-i",
            str(Path(ssh.privateKeyPath).expanduser()),
            "-o",
            f"StrictHostKeyChecking={ssh.strictHostKeyChecking}",
            "-o",
            f"ConnectTimeout={ssh.connectTimeoutSeconds}",
        ]

    def run(self, node: NodeConfig, command: str, *, timeout: int | None = None) -> str:
        argv = ["ssh", *self.ssh_base_args(), self.ssh_target(node), command]
        result = self.runner.run(argv, timeout=timeout)
        return result.stdout

    def copy(self, source: Path, node: NodeConfig, destination: str) -> None:
        argv = ["scp", *self.ssh_base_args(), str(source), f"{self.ssh_target(node)}:{destination}"]
        self.runner.run(argv)

    def wait_for_ssh(self, node: NodeConfig, *, timeout_seconds: int = 300) -> None:
        started = time.time()
        while True:
            try:
                self.run(node, "true", timeout=15)
                return
            except CommandError:
                if time.time() - started >= timeout_seconds:
                    raise
                time.sleep(5)

