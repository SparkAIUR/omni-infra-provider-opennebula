#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE="${SCRIPT_DIR}/../templates/provider-config.yaml.tmpl"

: "${ONE_ENDPOINT:?set ONE_ENDPOINT}"
: "${ONE_TEMPLATE_NAME:?set ONE_TEMPLATE_NAME}"
: "${ONE_ALLOWED_NETWORKS:=}"
: "${ONE_ALLOWED_DATASTORES:=}"

python3 - <<'PY' "$TEMPLATE"
from pathlib import Path
import os
import sys

template = Path(sys.argv[1]).read_text()
networks = os.environ.get("ONE_ALLOWED_NETWORKS", "")
datastores = os.environ.get("ONE_ALLOWED_DATASTORES", "")

def block(values: str) -> str:
    items = [item.strip() for item in values.split(",") if item.strip()]
    if not items:
        return "    []"
    return "\n".join(f'    - "{item}"' for item in items)

output = (
    template
    .replace("__ONE_ENDPOINT__", os.environ["ONE_ENDPOINT"])
    .replace("__ONE_TEMPLATE_NAME__", os.environ["ONE_TEMPLATE_NAME"])
    .replace("__ONE_ALLOWED_NETWORKS__", block(networks))
    .replace("__ONE_ALLOWED_DATASTORES__", block(datastores))
)
print(output)
PY
