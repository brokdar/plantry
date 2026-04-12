#!/usr/bin/env python
"""PostToolUse hook: prettier + eslint on edited TS/TSX files."""
import json
import subprocess
import sys
from pathlib import Path

data = json.load(sys.stdin)
file_path = Path(data.get("tool_input", {}).get("file_path", ""))

if file_path.suffix not in {".ts", ".tsx"}:
    sys.exit(0)

frontend_dir = Path(__file__).parent.parent.parent / "frontend"

subprocess.run(
    ["bunx", "prettier", "--write", str(file_path)],
    cwd=frontend_dir,
    check=False,
)

result = subprocess.run(
    ["bunx", "eslint", "--fix", str(file_path)],
    cwd=frontend_dir,
    capture_output=True,
    text=True,
)
if result.returncode != 0:
    print(result.stderr or result.stdout, file=sys.stderr)
    sys.exit(1)
