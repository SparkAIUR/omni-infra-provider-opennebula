"""SQLite-backed context store for concise project knowledge."""

from __future__ import annotations

import hashlib
import sqlite3
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path
from typing import Iterable


def repo_root() -> Path:
    """Return the repository root from the package location."""

    return Path(__file__).resolve().parents[4]


DEFAULT_DB_PATH = repo_root() / "refs" / "docs" / "ctx" / "context.db"
DEFAULT_KB_PATH = repo_root() / "refs" / "KB.md"


@dataclass(slots=True)
class Entry:
    """Context entry stored in SQLite."""

    id: int
    title: str
    body: str
    category: str
    tags: str
    source: str
    importance: int
    content_hash: str
    created_at: str
    updated_at: str


@dataclass(slots=True)
class EntryInput:
    """Payload used for creation and update."""

    title: str
    body: str
    category: str = "note"
    tags: str = ""
    source: str = "manual"
    importance: int = 3


class ContextStore:
    """High-level wrapper around the SQLite context database.

    Example:
        >>> store = ContextStore()
        >>> entry_id = store.add(EntryInput(title="Rule", body="Prefer session auth."))
        >>> len(store.search("session")) >= 1
        True
    """

    def __init__(self, db_path: Path = DEFAULT_DB_PATH) -> None:
        self.db_path = db_path
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self.connection = sqlite3.connect(self.db_path)
        self.connection.row_factory = sqlite3.Row
        self.connection.execute("PRAGMA foreign_keys = ON")
        self.connection.execute("PRAGMA journal_mode = WAL")
        self._init_schema()

    def close(self) -> None:
        """Close the underlying SQLite connection."""

        self.connection.close()

    def _init_schema(self) -> None:
        self.connection.executescript(
            """
            CREATE TABLE IF NOT EXISTS entries (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                title TEXT NOT NULL,
                body TEXT NOT NULL,
                category TEXT NOT NULL,
                tags TEXT NOT NULL DEFAULT '',
                source TEXT NOT NULL DEFAULT 'manual',
                importance INTEGER NOT NULL DEFAULT 3,
                content_hash TEXT NOT NULL UNIQUE,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );

            CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
                title,
                body,
                tags,
                content='entries',
                content_rowid='id'
            );

            CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON entries BEGIN
                INSERT INTO entries_fts(rowid, title, body, tags)
                VALUES (new.id, new.title, new.body, new.tags);
            END;

            CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
                INSERT INTO entries_fts(entries_fts, rowid, title, body, tags)
                VALUES ('delete', old.id, old.title, old.body, old.tags);
            END;

            CREATE TRIGGER IF NOT EXISTS entries_au AFTER UPDATE ON entries BEGIN
                INSERT INTO entries_fts(entries_fts, rowid, title, body, tags)
                VALUES ('delete', old.id, old.title, old.body, old.tags);
                INSERT INTO entries_fts(rowid, title, body, tags)
                VALUES (new.id, new.title, new.body, new.tags);
            END;
            """
        )
        self.connection.commit()

    def add(self, entry: EntryInput) -> int:
        """Insert a new entry or return the existing deduplicated row id."""

        now = datetime.now(UTC).isoformat()
        content_hash = self._hash(entry.title, entry.body, entry.category, entry.tags, entry.source)
        row = self.connection.execute(
            """
            SELECT id
            FROM entries
            WHERE content_hash = ?
            """,
            (content_hash,),
        ).fetchone()
        if row is not None:
            return int(row["id"])

        cursor = self.connection.execute(
            """
            INSERT INTO entries (
                title, body, category, tags, source, importance, content_hash, created_at, updated_at
            )
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                entry.title,
                entry.body,
                entry.category,
                entry.tags,
                entry.source,
                entry.importance,
                content_hash,
                now,
                now,
            ),
        )
        self.connection.commit()
        return int(cursor.lastrowid)

    def get(self, entry_id: int) -> Entry | None:
        """Return a single entry by id."""

        row = self.connection.execute(
            "SELECT * FROM entries WHERE id = ?",
            (entry_id,),
        ).fetchone()
        return self._row_to_entry(row) if row is not None else None

    def update(self, entry_id: int, entry: EntryInput) -> Entry:
        """Replace an existing entry."""

        current = self.get(entry_id)
        if current is None:
            raise KeyError(f"entry {entry_id} not found")

        updated_at = datetime.now(UTC).isoformat()
        content_hash = self._hash(entry.title, entry.body, entry.category, entry.tags, entry.source)
        self.connection.execute(
            """
            UPDATE entries
            SET title = ?, body = ?, category = ?, tags = ?, source = ?, importance = ?,
                content_hash = ?, updated_at = ?
            WHERE id = ?
            """,
            (
                entry.title,
                entry.body,
                entry.category,
                entry.tags,
                entry.source,
                entry.importance,
                content_hash,
                updated_at,
                entry_id,
            ),
        )
        self.connection.commit()
        refreshed = self.get(entry_id)
        assert refreshed is not None
        return refreshed

    def search(self, query: str, limit: int = 10) -> list[Entry]:
        """Search title, body, and tags using FTS5."""

        rows = self.connection.execute(
            """
            SELECT e.*
            FROM entries_fts f
            JOIN entries e ON e.id = f.rowid
            WHERE entries_fts MATCH ?
            ORDER BY rank, e.importance DESC, e.updated_at DESC
            LIMIT ?
            """,
            (query, limit),
        ).fetchall()
        return [self._row_to_entry(row) for row in rows]

    def list_recent(self, limit: int = 10) -> list[Entry]:
        """Return recent entries ordered by importance and update time."""

        rows = self.connection.execute(
            """
            SELECT *
            FROM entries
            ORDER BY importance DESC, updated_at DESC, id DESC
            LIMIT ?
            """,
            (limit,),
        ).fetchall()
        return [self._row_to_entry(row) for row in rows]

    def compact(self) -> None:
        """Run SQLite maintenance operations."""

        self.connection.execute("INSERT INTO entries_fts(entries_fts) VALUES ('optimize')")
        self.connection.execute("VACUUM")
        self.connection.commit()

    def write_digest(self, destination: Path = DEFAULT_KB_PATH, limit: int = 8) -> Path:
        """Refresh the generated digest section inside the tracked knowledge base."""

        destination.parent.mkdir(parents=True, exist_ok=True)
        content = destination.read_text(encoding="utf-8")
        digest = self._render_digest(self.list_recent(limit=limit))
        start_marker = "<!-- ctx:digest:start -->"
        end_marker = "<!-- ctx:digest:end -->"
        if start_marker not in content or end_marker not in content:
            raise ValueError("KB digest markers are missing")

        prefix, remainder = content.split(start_marker, maxsplit=1)
        _, suffix = remainder.split(end_marker, maxsplit=1)
        updated = f"{prefix}{start_marker}\n{digest}\n{end_marker}{suffix}"
        destination.write_text(updated, encoding="utf-8")
        return destination

    def _render_digest(self, entries: Iterable[Entry]) -> str:
        lines = []
        for entry in entries:
            body_preview = " ".join(entry.body.split())
            preview = body_preview[:140].rstrip()
            if len(body_preview) > 140:
                preview += "..."
            lines.append(
                f"- #{entry.id} [{entry.category}] {entry.title}: {preview} "
                f"(tags: {entry.tags or 'none'}, source: {entry.source})"
            )
        if not lines:
            return "No curated context-store entries yet."
        return "\n".join(lines)

    def _hash(self, title: str, body: str, category: str, tags: str, source: str) -> str:
        payload = "\n".join([title.strip(), body.strip(), category.strip(), tags.strip(), source.strip()])
        return hashlib.sha256(payload.encode("utf-8")).hexdigest()

    def _row_to_entry(self, row: sqlite3.Row) -> Entry:
        return Entry(
            id=int(row["id"]),
            title=str(row["title"]),
            body=str(row["body"]),
            category=str(row["category"]),
            tags=str(row["tags"]),
            source=str(row["source"]),
            importance=int(row["importance"]),
            content_hash=str(row["content_hash"]),
            created_at=str(row["created_at"]),
            updated_at=str(row["updated_at"]),
        )
