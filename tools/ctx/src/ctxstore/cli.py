"""Command line interface for the private context store."""

from __future__ import annotations

from pathlib import Path

import typer

from .store import DEFAULT_DB_PATH, DEFAULT_KB_PATH, ContextStore, EntryInput

app = typer.Typer(help="Private contextual knowledge store for this repository.")


def _store(db_path: Path) -> ContextStore:
    return ContextStore(db_path=db_path)


@app.command()
def add(
    title: str = typer.Option(..., help="Short entry title."),
    body: str = typer.Option(..., help="Entry body text."),
    category: str = typer.Option("note", help="Entry category."),
    tags: str = typer.Option("", help="Comma-separated tags."),
    source: str = typer.Option("manual", help="Source of the knowledge."),
    importance: int = typer.Option(3, min=1, max=5, help="Relative importance from 1 to 5."),
    db_path: Path = typer.Option(DEFAULT_DB_PATH, "--db", help="SQLite database path."),
) -> None:
    """Add a new knowledge entry, deduplicating identical content."""

    store = _store(db_path)
    try:
        entry_id = store.add(
            EntryInput(
                title=title,
                body=body,
                category=category,
                tags=tags,
                source=source,
                importance=importance,
            )
        )
    finally:
        store.close()

    typer.echo(entry_id)


@app.command()
def search(
    query: str,
    limit: int = typer.Option(10, min=1, max=50, help="Maximum number of results."),
    db_path: Path = typer.Option(DEFAULT_DB_PATH, "--db", help="SQLite database path."),
) -> None:
    """Search the context store using the FTS index."""

    store = _store(db_path)
    try:
        results = store.search(query, limit=limit)
    finally:
        store.close()

    for entry in results:
        typer.echo(f"#{entry.id} [{entry.category}] {entry.title} | tags={entry.tags or 'none'}")


@app.command()
def show(
    entry_id: int,
    db_path: Path = typer.Option(DEFAULT_DB_PATH, "--db", help="SQLite database path."),
) -> None:
    """Show a single entry in full."""

    store = _store(db_path)
    try:
        entry = store.get(entry_id)
    finally:
        store.close()

    if entry is None:
        raise typer.Exit(code=1)

    typer.echo(f"#{entry.id} {entry.title}")
    typer.echo(f"category: {entry.category}")
    typer.echo(f"tags: {entry.tags or 'none'}")
    typer.echo(f"source: {entry.source}")
    typer.echo(f"importance: {entry.importance}")
    typer.echo(f"created: {entry.created_at}")
    typer.echo(f"updated: {entry.updated_at}")
    typer.echo("")
    typer.echo(entry.body)


@app.command()
def update(
    entry_id: int,
    title: str = typer.Option(..., help="Short entry title."),
    body: str = typer.Option(..., help="Entry body text."),
    category: str = typer.Option("note", help="Entry category."),
    tags: str = typer.Option("", help="Comma-separated tags."),
    source: str = typer.Option("manual", help="Source of the knowledge."),
    importance: int = typer.Option(3, min=1, max=5, help="Relative importance from 1 to 5."),
    db_path: Path = typer.Option(DEFAULT_DB_PATH, "--db", help="SQLite database path."),
) -> None:
    """Update an existing knowledge entry."""

    store = _store(db_path)
    try:
        entry = store.update(
            entry_id,
            EntryInput(
                title=title,
                body=body,
                category=category,
                tags=tags,
                source=source,
                importance=importance,
            ),
        )
    finally:
        store.close()

    typer.echo(f"updated #{entry.id}")


@app.command()
def digest(
    kb_path: Path = typer.Option(DEFAULT_KB_PATH, "--kb", help="Knowledge-base markdown path."),
    limit: int = typer.Option(8, min=1, max=50, help="Number of items to include."),
    db_path: Path = typer.Option(DEFAULT_DB_PATH, "--db", help="SQLite database path."),
) -> None:
    """Refresh the generated digest section inside `refs/KB.md`."""

    store = _store(db_path)
    try:
        path = store.write_digest(destination=kb_path, limit=limit)
    finally:
        store.close()

    typer.echo(path)


@app.command()
def compact(
    db_path: Path = typer.Option(DEFAULT_DB_PATH, "--db", help="SQLite database path."),
) -> None:
    """Run SQLite maintenance operations on the context store."""

    store = _store(db_path)
    try:
        store.compact()
    finally:
        store.close()

    typer.echo("ok")


def main() -> None:
    """Run the CLI application."""

    app()


if __name__ == "__main__":
    main()
