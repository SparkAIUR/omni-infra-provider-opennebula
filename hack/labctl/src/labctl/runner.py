from __future__ import annotations

import os
import shlex
import subprocess
import time
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path


@dataclass
class CommandResult:
    argv: list[str]
    returncode: int
    stdout: str
    stderr: str


class CommandError(RuntimeError):
    def __init__(self, message: str, result: CommandResult | None = None) -> None:
        super().__init__(message)
        self.result = result


class Runner:
    def __init__(self, log_dir: Path, secrets: Sequence[str] | None = None) -> None:
        self.log_dir = log_dir
        self.log_dir.mkdir(parents=True, exist_ok=True)
        self.secrets = [secret for secret in (secrets or []) if secret]

    def redact(self, text: str) -> str:
        redacted = text
        for secret in self.secrets:
            redacted = redacted.replace(secret, "***REDACTED***")
        return redacted

    def run(
        self,
        argv: Sequence[str],
        *,
        env: Mapping[str, str] | None = None,
        cwd: Path | None = None,
        timeout: int | None = None,
        check: bool = True,
    ) -> CommandResult:
        process = subprocess.run(
            list(argv),
            cwd=str(cwd) if cwd else None,
            env={**os.environ, **(dict(env) if env else {})},
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
        result = CommandResult(
            argv=list(argv),
            returncode=process.returncode,
            stdout=process.stdout,
            stderr=process.stderr,
        )
        self._write_log(result)
        if check and process.returncode != 0:
            command = " ".join(shlex.quote(part) for part in argv)
            summary = self.redact(process.stderr.strip() or process.stdout.strip())
            raise CommandError(f"command failed: {command}\n{summary}", result=result)
        return result

    def retry(
        self,
        argv: Sequence[str],
        *,
        attempts: int,
        delay_seconds: float,
        env: Mapping[str, str] | None = None,
        cwd: Path | None = None,
        timeout: int | None = None,
    ) -> CommandResult:
        last_error: CommandError | None = None
        for attempt in range(1, attempts + 1):
            try:
                return self.run(argv, env=env, cwd=cwd, timeout=timeout)
            except CommandError as exc:
                last_error = exc
                if attempt == attempts:
                    break
                time.sleep(delay_seconds * attempt)
        raise last_error if last_error else RuntimeError("retry failed without error")

    def _write_log(self, result: CommandResult) -> None:
        stamp = int(time.time() * 1000)
        path = self.log_dir / f"{stamp}.log"
        body = [
            "$ " + " ".join(shlex.quote(part) for part in result.argv),
            "",
            "stdout:",
            self.redact(result.stdout),
            "",
            "stderr:",
            self.redact(result.stderr),
            "",
            f"returncode: {result.returncode}",
        ]
        path.write_text("\n".join(body) + "\n")
