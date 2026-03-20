from __future__ import annotations

import json
from dataclasses import dataclass, field
from datetime import UTC, datetime
from pathlib import Path
from typing import Any

PHASE_PENDING = "pending"
PHASE_RUNNING = "running"
PHASE_COMPLETED = "completed"
PHASE_FAILED = "failed"
PHASE_SKIPPED = "skipped"


def utc_now() -> str:
    return datetime.now(UTC).isoformat()


@dataclass
class PhaseRecord:
    status: str = PHASE_PENDING
    config_hash: str | None = None
    started_at: str | None = None
    completed_at: str | None = None
    details: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "status": self.status,
            "config_hash": self.config_hash,
            "started_at": self.started_at,
            "completed_at": self.completed_at,
            "details": self.details,
        }


class StateStore:
    def __init__(self, path: Path) -> None:
        self.path = path
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.data: dict[str, Any] = self._load()

    def _load(self) -> dict[str, Any]:
        if not self.path.exists():
            return {"phases": {}, "facts": {}, "artifacts": {}}
        return json.loads(self.path.read_text())

    def save(self) -> None:
        self.path.write_text(json.dumps(self.data, indent=2, sort_keys=True) + "\n")

    def phase(self, name: str) -> PhaseRecord:
        phases = self.data.setdefault("phases", {})
        raw = phases.get(name, {})
        return PhaseRecord(
            status=raw.get("status", PHASE_PENDING),
            config_hash=raw.get("config_hash"),
            started_at=raw.get("started_at"),
            completed_at=raw.get("completed_at"),
            details=raw.get("details", {}),
        )

    def update_phase(
        self,
        name: str,
        *,
        status: str,
        config_hash: str | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        record = self.phase(name)
        if status == PHASE_RUNNING:
            record.started_at = utc_now()
        if status in {PHASE_COMPLETED, PHASE_FAILED, PHASE_SKIPPED}:
            record.completed_at = utc_now()
        record.status = status
        if config_hash is not None:
            record.config_hash = config_hash
        if details is not None:
            record.details = details
        self.data.setdefault("phases", {})[name] = record.to_dict()
        self.save()

    def update_fact(self, key: str, value: Any) -> None:
        self.data.setdefault("facts", {})[key] = value
        self.save()

    def fact(self, key: str, default: Any | None = None) -> Any:
        return self.data.setdefault("facts", {}).get(key, default)

    def update_artifact(self, key: str, value: Any) -> None:
        self.data.setdefault("artifacts", {})[key] = value
        self.save()

    def first_incomplete(self, ordered_phases: list[str]) -> str | None:
        for phase_name in ordered_phases:
            if self.phase(phase_name).status not in {PHASE_COMPLETED, PHASE_SKIPPED}:
                return phase_name
        return None
