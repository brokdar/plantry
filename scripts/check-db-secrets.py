#!/usr/bin/env python3
"""Fail when a SQLite database carries credentials.

Run as `python3 -I scripts/check-db-secrets.py [--tracked | FILE...]`.
Checks, per DB: an app_settings row with encrypted=1, a setting value matching
a `value` pattern, a credential pattern in the raw bytes (covers deleted rows
that were never VACUUMed), and a nonempty freelist. Patterns live in
scripts/secret-patterns.txt, shared with the Go guard test. Never prints a
secret value, only file, key and pattern.
"""
import os
import re
import sqlite3
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DB_RE = re.compile(r"\.(db|sqlite[0-9]*)$")


def load_patterns():
    raw, value = [], []
    with open(os.path.join(ROOT, "scripts", "secret-patterns.txt"), encoding="utf-8") as f:
        for line in f:
            line = line.rstrip("\n")
            if not line.strip() or line.startswith("#"):
                continue
            scope, pat = line.split("\t", 1)
            (raw if scope == "raw" else value).append(re.compile(pat.encode() if scope == "raw" else pat))
    return raw, value


def check(path, raw_pats, value_pats):
    problems = []
    with open(path, "rb") as f:
        data = f.read()
    if not data.startswith(b"SQLite format 3\x00"):
        return [f"{path}: not a SQLite database"]
    for p in raw_pats:
        if p.search(data):
            problems.append(f"{path}: raw bytes match credential pattern {p.pattern.decode()!r} (deleted row not VACUUMed?)")
    if int.from_bytes(data[36:40], "big") > 0:
        problems.append(f"{path}: freelist is not empty; VACUUM the fixture so deleted rows leave no residue")
    conn = sqlite3.connect(f"file:{path}?mode=ro&immutable=1", uri=True)
    try:
        try:
            rows = conn.execute("SELECT key, value, encrypted FROM app_settings").fetchall()
        except sqlite3.OperationalError as e:
            if "no such table" not in str(e):
                raise
            rows = []
        for key, value, encrypted in rows:
            if encrypted:
                problems.append(f"{path}: app_settings {key!r} is encrypted; remove it from the fixture")
            for p in value_pats:
                if isinstance(value, str) and p.search(value):
                    problems.append(f"{path}: app_settings {key!r} looks like a credential ({p.pattern!r})")
    finally:
        conn.close()
    return problems


def main(argv):
    if argv == ["--tracked"]:
        out = subprocess.run(["git", "-C", ROOT, "ls-files", "-z"], check=True, capture_output=True).stdout
        files = [os.path.join(ROOT, n) for n in out.decode().split("\0") if DB_RE.search(n)]
    else:
        files = argv
    raw_pats, value_pats = load_patterns()
    problems = []
    for f in files:
        problems += check(f, raw_pats, value_pats)
    for p in problems:
        print(p, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
