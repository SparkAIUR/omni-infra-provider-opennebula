#!/usr/bin/env bash
set -euo pipefail

echo "== templates =="
onetemplate list
echo
echo "== images =="
oneimage list
echo
echo "== networks =="
onevnet list
