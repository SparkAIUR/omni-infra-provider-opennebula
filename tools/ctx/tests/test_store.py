from __future__ import annotations

from pathlib import Path

from ctxstore.store import ContextStore, EntryInput


def make_store(tmp_path: Path) -> ContextStore:
    return ContextStore(db_path=tmp_path / "context.db")


def test_add_deduplicates_identical_entries(tmp_path: Path) -> None:
    store = make_store(tmp_path)
    try:
        first = store.add(EntryInput(title="Auth", body="Prefer session auth.", tags="auth"))
        second = store.add(EntryInput(title="Auth", body="Prefer session auth.", tags="auth"))
        assert first == second
    finally:
        store.close()


def test_search_returns_matching_entries(tmp_path: Path) -> None:
    store = make_store(tmp_path)
    try:
        store.add(EntryInput(title="OpenNebula", body="Use GOCA for VM lifecycle.", tags="goca"))
        results = store.search("OpenNebula")
        assert len(results) == 1
        assert results[0].title == "OpenNebula"
    finally:
        store.close()


def test_update_rewrites_entry(tmp_path: Path) -> None:
    store = make_store(tmp_path)
    try:
        entry_id = store.add(EntryInput(title="State", body="Initial", tags="state"))
        updated = store.update(
            entry_id,
            EntryInput(title="State", body="Updated body", category="decision", tags="state,proto"),
        )
        assert updated.body == "Updated body"
        assert updated.category == "decision"
    finally:
        store.close()


def test_digest_writes_between_markers(tmp_path: Path) -> None:
    store = make_store(tmp_path)
    kb_path = tmp_path / "KB.md"
    kb_path.write_text(
        "# KB\n\n<!-- ctx:digest:start -->\nold\n<!-- ctx:digest:end -->\n",
        encoding="utf-8",
    )
    try:
        store.add(EntryInput(title="Retry", body="Treat missing VMs as already deleted.", tags="delete"))
        store.write_digest(destination=kb_path, limit=5)
    finally:
        store.close()

    content = kb_path.read_text(encoding="utf-8")
    assert "Treat missing VMs as already deleted." in content
