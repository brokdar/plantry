#!/usr/bin/env python
"""PreToolUse hook: block manual edits to go.sum and bun.lock."""
import json
import sys
from pathlib import Path

PROTECTED = {"go.sum", "bun.lock"}

data = json.load(sys.stdin)
file_path = Path(data.get("tool_input", {}).get("file_path", ""))

MESSAGES = {
    "go.sum": "Blocked: go.sum is managed by Go modules. Run 'go mod tidy' instead.",
    "bun.lock": "Blocked: bun.lock is managed by bun. Run 'bun install' instead.",
}

if file_path.name in PROTECTED:
    print(MESSAGES[file_path.name])
    sys.exit(2)
