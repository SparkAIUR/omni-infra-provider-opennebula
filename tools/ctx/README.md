# Context Store CLI

Internal SQLite-backed context store for concise, queryable project knowledge.

## Commands

```bash
uv run --project tools/ctx ctx --help
uv run --project tools/ctx ctx add --title "OpenNebula auth" --body "Prefer OPENNEBULA_SESSION over username/password."
uv run --project tools/ctx ctx search opennebula
uv run --project tools/ctx ctx digest
```

## Storage

- Database: `refs/docs/ctx/context.db`
- Tracked digest target: `refs/KB.md`

The database is private and gitignored. The digest writes a concise generated section into the tracked knowledge base.
