#!/usr/bin/env python3
"""Fail when a committed file can carry credentials that gitleaks cannot read.

Run as `python3 -I scripts/check-db-secrets.py MODE`, MODE being one of
  FILE...              working-tree files (pre-commit: the staged files)
  --tracked            every file in the index (gate, CI)
  --history REVARGS... every file added or changed in `git log -m REVARGS`
                       (pre-push, gate, CI); merge commits are included, so a
                       file only an evil merge touches is checked

Per file (the repo path decides, never trust a name alone):
  * blocked names, case-insensitive: .env*, secret.key, keys, ssh keys, local
    Claude settings, and database names (.db .db3 .sqlite* .sdb .s3db -wal
    -shm -journal) outside a testdata directory;
  * SQLite magic on ANY file: must sit under testdata, then the DB checks run
    (encrypted app_settings row, key-like value, credential pattern in the raw
    bytes which covers deleted rows, non-empty freelist);
  * other binary files (a NUL in the first 8000 bytes): credential patterns in
    the raw bytes (also with NULs removed, which reads UTF-16), archives
    rejected, unknown binaries rejected unless the path is listed, with a
    reason, in scripts/binary-allowlist.txt. PNG/JPEG/GIF/WebP/ICO/fonts pass.

Patterns live in scripts/secret-patterns.txt, shared with the Go guard test.
Never prints a secret value, only file, key and pattern.

SECRET_GUARD_REPO names the repo to scan when the script runs from a trusted
copy of the guard files outside it (CI runs the scripts from origin/main).
"""
import fnmatch
import os
import re
import sqlite3
import subprocess
import sys
import tempfile
import threading

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
REPO = os.environ.get("SECRET_GUARD_REPO") or ROOT
SQLITE_MAGIC = b"SQLite format 3\x00"

DB_NAME_RE = re.compile(r"\.(db|db3|sdb|s3db|sqlite[0-9]*)(-wal|-shm|-journal)?$")
CRED_NAMES = re.compile(r"^(\.env|\.env\..*|secret\.key|.*\.pem|.*\.p12|.*\.pfx|.*\.jks|.*\.keystore|.*\.key|id_rsa.*|id_ed25519.*)$")

ARCHIVES = [
    (b"\x1f\x8b", "gzip"),
    (b"PK\x03\x04", "zip"),
    (b"PK\x05\x06", "zip"),
    (b"\xfd7zXZ\x00", "xz"),
    (b"BZh", "bzip2"),
    (b"\x28\xb5\x2f\xfd", "zstd"),
    (b"7z\xbc\xaf\x27\x1c", "7z"),
    (b"Rar!", "rar"),
    (b"\x04\x22\x4d\x18", "lz4"),
]
SAFE_MAGIC = [
    b"\x89PNG\r\n\x1a\n",
    b"\xff\xd8\xff",
    b"GIF87a",
    b"GIF89a",
    b"\x00\x00\x01\x00",
    b"wOFF",
    b"wOF2",
    b"\x00\x01\x00\x00",
    b"OTTO",
]


def is_testdata(path):
    return path.startswith("testdata/") or "/testdata/" in path


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


def load_allowlist():
    """binary-allowlist.txt: `<glob>\\t<reason>`; a missing reason is an error."""
    entries = []
    with open(os.path.join(ROOT, "scripts", "binary-allowlist.txt"), encoding="utf-8") as f:
        for n, line in enumerate(f, 1):
            line = line.rstrip("\n")
            if not line.strip() or line.startswith("#"):
                continue
            glob, _, reason = line.partition("\t")
            if not reason.strip():
                raise SystemExit(f"scripts/binary-allowlist.txt:{n}: entry needs a reason after a TAB")
            entries.append(glob)
    return entries


def name_problems(path):
    base = os.path.basename(path).lower()
    low = path.lower()
    out = []
    if not base.endswith(".example") and CRED_NAMES.match(base):
        out.append(f"{path}: credential file name; never commit it")
    if low == ".claude/settings.local.json" or low.endswith("/.claude/settings.local.json"):
        out.append(f"{path}: local-only settings")
    if DB_NAME_RE.search(base) and not is_testdata(low):
        out.append(f"{path}: database outside a testdata directory")
    return out


def check_db(display, data, raw_pats, value_pats):
    problems = []
    for p in raw_pats:
        if p.search(data):
            problems.append(f"{display}: raw bytes match credential pattern {p.pattern.decode()!r} (deleted row not VACUUMed?)")
    if int.from_bytes(data[36:40], "big") > 0:
        problems.append(f"{display}: freelist is not empty; VACUUM the fixture so deleted rows leave no residue")
    with tempfile.TemporaryDirectory() as td:
        path = os.path.join(td, "db")
        with open(path, "wb") as f:
            f.write(data)
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
                    problems.append(f"{display}: app_settings {key!r} is encrypted; remove it from the fixture")
                for p in value_pats:
                    if isinstance(value, str) and p.search(value):
                        problems.append(f"{display}: app_settings {key!r} looks like a credential ({p.pattern!r})")
        finally:
            conn.close()
    return problems


def check_blob(path, data, raw_pats, value_pats, allow):
    """All problems for one file; `path` is the repo-relative display path."""
    problems = name_problems(path)
    if data.startswith(SQLITE_MAGIC):
        if not is_testdata(path.lower()) and not DB_NAME_RE.search(os.path.basename(path).lower()):
            problems.append(f"{path}: a SQLite database under a non-database name, outside testdata")
        return problems + check_db(path, data, raw_pats, value_pats)
    if b"\x00" not in data[:8000]:
        return problems
    for p in raw_pats:
        if p.search(data) or p.search(data.replace(b"\x00", b"")):
            problems.append(f"{path}: binary content matches credential pattern {p.pattern.decode()!r}")
    if any(fnmatch.fnmatchcase(path, g) for g in allow):
        return problems
    for magic, kind in ARCHIVES:
        if data.startswith(magic):
            problems.append(f"{path}: {kind} archive can hide credentials; list it in scripts/binary-allowlist.txt with a reason if it is reviewed")
            return problems
    if (data[:4] == b"RIFF" and data[8:12] == b"WEBP") or any(data.startswith(m) for m in SAFE_MAGIC):
        return problems
    problems.append(f"{path}: unknown binary file; list it in scripts/binary-allowlist.txt with a reason if it is reviewed")
    return problems


def git(*args):
    return subprocess.run(["git", "-C", REPO, *args], check=True, capture_output=True).stdout


def read_blobs(shas):
    """{sha: bytes} through one `git cat-file --batch`."""
    out = {}
    if not shas:
        return out
    proc = subprocess.Popen(["git", "-C", REPO, "cat-file", "--batch"], stdin=subprocess.PIPE, stdout=subprocess.PIPE)

    def feed():
        for s in shas:
            proc.stdin.write(s.encode() + b"\n")
        proc.stdin.close()

    t = threading.Thread(target=feed)
    t.start()
    for s in shas:
        head = proc.stdout.readline().split()
        if len(head) != 3 or head[1] != b"blob":
            out[s] = b""
            continue
        out[s] = proc.stdout.read(int(head[2]))
        proc.stdout.read(1)
    t.join()
    proc.wait()
    return out


def entries_index():
    entries = []
    for rec in git("ls-files", "-z", "-s").split(b"\0"):
        if not rec:
            continue
        meta, _, path = rec.partition(b"\t")
        mode, sha, _stage = meta.decode().split()
        if mode != "160000":
            entries.append((sha, path.decode("utf-8", "surrogateescape")))
    return entries


def entries_history(revargs):
    out = git("log", "-m", "--raw", "--no-abbrev", "--no-renames", "-z", "--diff-filter=AM", "--format=", *revargs)
    toks = out.split(b"\0")
    seen, entries = set(), []
    i = 0
    while i < len(toks):
        t = toks[i]
        if not t.startswith(b":"):
            i += 1
            continue
        meta = t.decode().split()
        path = toks[i + 1].decode("utf-8", "surrogateescape")
        i += 2
        if meta[1] == "160000" or meta[4].startswith("D"):
            continue
        key = (meta[3], path)
        if key not in seen:
            seen.add(key)
            entries.append(key)
    return entries


def main(argv):
    raw_pats, value_pats = load_patterns()
    allow = load_allowlist()
    problems = []
    if argv == ["--tracked"] or (argv and argv[0] == "--history"):
        entries = entries_index() if argv == ["--tracked"] else entries_history(argv[1:])
        blobs = read_blobs(sorted({s for s, _ in entries}))
        for sha, path in entries:
            problems += check_blob(path, blobs[sha], raw_pats, value_pats, allow)
    else:
        for f in argv:
            rel = os.path.relpath(os.path.abspath(f), REPO)
            if rel.startswith(".."):
                rel = f
            try:
                with open(f, "rb") as fh:
                    data = fh.read()
            except (IsADirectoryError, FileNotFoundError):
                continue
            problems += check_blob(rel, data, raw_pats, value_pats, allow)
    for p in problems:
        print(p, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
