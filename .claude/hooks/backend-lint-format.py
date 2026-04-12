#!/usr/bin/env python
"""PostToolUse hook: gofmt + go vet on edited Go files."""
import json
import subprocess
import sys
from pathlib import Path

data = json.load(sys.stdin)
file_path = Path(data.get("tool_input", {}).get("file_path", ""))

if file_path.suffix != ".go":
    sys.exit(0)

backend_dir = Path(__file__).parent.parent.parent / "backend"

subprocess.run(["gofmt", "-w", str(file_path)], check=False)

result = subprocess.run(
    ["go", "vet", "./..."],
    cwd=backend_dir,
    capture_output=True,
    text=True,
)
if result.returncode != 0:
    print(result.stderr, file=sys.stderr)
    sys.exit(1)
