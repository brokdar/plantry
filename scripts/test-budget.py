#!/usr/bin/env python3
"""Test-time ratchet: tests may get slower only in proportion to the tests
added, or with a written reason. Budgets live in test-budget.json (repo root).

    test-budget.py go NORACE_JSON CPU_LOG [--write]      Go tests, no race
    test-budget.py race RACE_JSON RACE_CPU_LOG [--write] the -race -short tier
    test-budget.py vitest VITEST_JSON [--write]
    test-budget.py e2e PLAYWRIGHT_JSON [--write]
    test-budget.py timeouts            every timeout knob equals test-budget.json
    test-budget.py slowlist            every testhelper.Slow test is listed
    test-budget.py diff [REF]          raises against REF (origin/main) carry a new reason

Budget rule, per Go package (CPU seconds of its test binary, no race), for
the race tier as a whole, and for vitest / e2e (wall seconds):

    allowed = budget * (1 + tolerance) + 1 s + max(0, tests - budget_tests) * budget / budget_tests

New tests buy their average share. Anything above fails. The only way past
is a higher budget in the same PR with a reason under "reasons" that says why
it costs this much and why it can't be faster; `diff` enforces the reason.
`--write` lowers budgets that are beaten (the ratchet) and adds missing
entries; it never raises one.

Numbers are per machine: "ci" when $CI is set (GitHub's 4-vCPU runner, the
reference), else "local". A machine without numbers yet is reported, not
checked; --write seeds it (CI prints the numbers to copy into the file).
"""
import json
import os
import re
import subprocess
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BUDGET = os.path.join(ROOT, "test-budget.json")
MODULE = "github.com/jaltszeimer/plantry/backend/"
NEW_ENTRY_FREE = 5.0  # a new package or entry under this needs no reason
ABS_SLACK = 1.0  # seconds of noise any entry may add (budgets are rounded to 0.1 s)

TIMEOUT_FILES = {
    ".github/workflows/backend.yml": [r"timeout-minutes:\s*(\d+)", r"-timeout\s+(\d+[smh])"],
    ".github/workflows/frontend.yml": [r"timeout-minutes:\s*(\d+)"],
    ".github/workflows/e2e.yml": [r"timeout-minutes:\s*(\d+)"],
    ".github/workflows/docker.yml": [r"timeout-minutes:\s*(\d+)"],
    "scripts/gate.sh": [r"-timeout\s+(\d+[smh])"],
    "frontend/playwright.config.ts": [r"\btimeout:\s*([\d_]+)"],
    "frontend/vite.config.ts": [r"\b(?:testTimeout|hookTimeout):\s*([\d_]+)"],
}

failures, notes = [], []
SEEDING = False  # this machine has no budgets yet: report only


def machine():
    return "ci" if os.environ.get("CI") else "local"


def load(path=BUDGET):
    with open(path, encoding="utf-8") as f:
        return json.load(f)


def save(b):
    with open(BUDGET, "w", encoding="utf-8") as f:
        json.dump(b, f, indent=2, sort_keys=True)
        f.write("\n")


def allowed(entry, tests, tol, key="cpu_s"):
    base, n = entry[key], max(entry.get("tests", 0), 1)
    return base * (1 + tol) + ABS_SLACK + max(0, tests - entry.get("tests", 0)) * base / n


def check_entry(section, name, entry, value, tests, tol, key, write, table):
    if entry is None:
        if value > NEW_ENTRY_FREE and not write and not SEEDING:
            failures.append(f"{section} {name}: {value:.1f}s has no budget; run with --write and add a reason if it is over {NEW_ENTRY_FREE:.0f}s")
        table.append((name, value, None, None, tests))
        return {"tests": tests, key: round(value, 1)} if write else None
    lim = allowed(entry, tests, tol, key)
    table.append((name, value, entry[key], lim, tests))
    if value > lim:
        failures.append(
            f"{section} {name}: {value:.1f}s > allowed {lim:.1f}s (budget {entry[key]}s for {entry.get('tests')} tests, now {tests}). "
            "Make it faster. A raise needs a reason in test-budget.json (why it is slow, why it can't be faster)."
        )
    elif value < entry[key] * 0.6:
        notes.append(f"{section} {name}: {value:.1f}s is far under its budget {entry[key]}s; run --write to lock in the gain")
    if write and value < entry[key]:
        return {"tests": tests, key: round(value, 1)}
    return None


def go_tests(path):
    """Top-level tests that ran, per package, and the elapsed seconds of every
    leaf test (a test without subtests). A parent's time adds up subtests
    that run in parallel, so the per-test cap applies to the leaves."""
    counts, elapsed = {}, {}
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                ev = json.loads(line)
            except ValueError:
                continue
            test, pkg = ev.get("Test"), ev.get("Package", "")
            if not test or ev.get("Action") not in ("pass", "fail"):
                continue
            rel = pkg[len(MODULE):] if pkg.startswith(MODULE) else pkg
            if "/" not in test:
                counts[rel] = counts.get(rel, 0) + 1
            elapsed[f"{rel}.{test}"] = ev.get("Elapsed", 0.0)
    parents = {name.rsplit("/", 1)[0] for name in elapsed if "/" in name.split(".", 1)[-1]}
    leaves = {k: v for k, v in elapsed.items() if k not in parents}
    return counts, leaves


def go_cpu(path):
    cpu = {}
    backend = os.path.join(ROOT, "backend")
    if not os.path.exists(path):
        return cpu
    with open(path, encoding="utf-8") as f:
        for line in f:
            r = json.loads(line)
            rel = os.path.relpath(r["dir"], os.path.realpath(backend))
            if rel.startswith(".."):  # another checkout of the repo
                rel = r["dir"].split("/backend/", 1)[-1]
            cpu[rel] = cpu.get(rel, 0.0) + r["cpu_s"]
    return cpu


def summary(title, table, key="s"):
    rows = sorted(table, key=lambda r: -(r[1] / r[3] if r[3] else 0))
    out = [f"### {title} ({machine()})", "", "| | used | budget | allowed | tests |", "|---|---:|---:|---:|---:|"]
    for name, v, b, lim, n in rows[:15]:
        out.append(f"| {name} | {v:.1f}{key} | {b if b is not None else '-'} | {f'{lim:.1f}' if lim else '-'} | {n} |")
    text = "\n".join(out) + "\n"
    print(text)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as f:
            f.write(text)


def machine_budgets(b, write):
    """The budgets of this machine; a machine without any is seeded."""
    m = machine()
    if m not in b["machines"]:
        # Nothing to check against yet: report, never fail. --write stores
        # the numbers (locally; CI prints them to copy into the file).
        notes.append(f"no budgets for machine {m!r} yet: nothing checked; run with --write to seed")
        b["machines"][m] = {}
        global SEEDING
        SEEDING = True
    return b["machines"][m], write


def cmd_go(args):
    write = "--write" in args
    args = [a for a in args if a != "--write"]
    b = load()
    m, write = machine_budgets(b, write)
    tol = b["tolerance"]["cpu"]
    counts, elapsed = go_tests(args[0])
    cpu = go_cpu(args[1])
    budgets = m.setdefault("go", {})
    table = []
    for pkg, v in sorted(cpu.items()):
        n = counts.get(pkg, 0)
        if n == 0:
            continue
        upd = check_entry("go", pkg, budgets.get(pkg), v, n, tol, "cpu_s", write, table)
        if upd:
            budgets[pkg] = upd
    summary("Go test CPU per package, no race", table)
    cap, slow = b["single_test_max_s"].get(machine(), b["single_test_max_s"]["local"]), b.get("slow_tests", {})
    for name, secs in sorted(elapsed.items(), key=lambda x: -x[1]):
        pkg, test = name.split(".", 1) if "." in name else ("", name)
        if secs > cap and f"{pkg}.{test.split('/', 1)[0]}" not in slow:
            failures.append(f"{name}: {secs:.1f}s > {cap}s for one test. Make it faster, or mark it testhelper.Slow with a reason and list it under slow_tests.")
    finish(b, write)


def cmd_race(args):
    write = "--write" in args
    args = [a for a in args if a != "--write"]
    b = load()
    m, write = machine_budgets(b, write)
    rc = sum(go_cpu(args[1]).values())
    rn = sum(go_tests(args[0])[0].values())
    table = []
    upd = check_entry("go_race", "race tier (-race -short)", m.get("go_race"), rc, rn, b["tolerance"]["cpu"], "cpu_s", write, table)
    if upd:
        m["go_race"] = upd
    summary("Go race tier CPU", table)
    finish(b, write)


def finish(b, write):
    if SEEDING and os.environ.get("CI"):
        write = True  # print this machine's numbers to seed its budgets
    if not write:
        return
    if os.environ.get("CI"):
        # CI cannot commit: print this machine's numbers to copy into the file.
        text = json.dumps({machine(): b["machines"][machine()]}, indent=2, sort_keys=True)
        print("budgets measured on this machine:\n" + text)
        if os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as f:
                f.write("```json\n" + text + "\n```\n")
        return
    save(b)


def cmd_wall(section, args, parse):
    write = "--write" in args
    b = load()
    m, write = machine_budgets(b, write)
    wall, n = parse(args[0])
    table = []
    upd = check_entry(section, section, m.get(section), wall, n, b["tolerance"]["wall"], "wall_s", write, table)
    if upd:
        m[section] = upd
    summary(f"{section} wall time", table)
    finish(b, write)


def parse_vitest(path):
    d = load(path)
    start = d.get("startTime", 0) / 1000
    end = max((r.get("endTime", 0) for r in d.get("testResults", [])), default=0) / 1000
    return max(end - start, 0.0), d.get("numPassedTests", 0) + d.get("numFailedTests", 0)


def parse_e2e(path):
    d = load(path)
    s = d["stats"]
    return s["duration"] / 1000, s.get("expected", 0) + s.get("unexpected", 0) + s.get("flaky", 0)


def timeouts_now():
    out = {}
    for rel, pats in TIMEOUT_FILES.items():
        p = os.path.join(ROOT, rel)
        if not os.path.exists(p):
            continue
        text = open(p, encoding="utf-8").read()
        vals = []
        for pat in pats:
            vals += re.findall(pat, text)
        out[rel] = vals
    return out


def cmd_timeouts(_):
    want = load().get("timeouts", {})
    now = timeouts_now()
    for rel in sorted(set(want) | set(now)):
        if want.get(rel, []) != now.get(rel, []):
            failures.append(
                f"timeouts in {rel}: {now.get(rel, [])} but test-budget.json says {want.get(rel, [])}. "
                "A timeout is a tripwire for hangs, not a fix for slow tests: profile and fix the test. "
                "A deliberate change goes into test-budget.json with a reason (timeouts:<file>)."
            )
    if not failures:
        print("timeouts: all match test-budget.json")


def cmd_slowlist(_):
    listed = load().get("slow_tests", {})
    found = set()
    backend = os.path.join(ROOT, "backend")
    for dirpath, _, files in os.walk(backend):
        for fn in files:
            if not fn.endswith("_test.go"):
                continue
            text = open(os.path.join(dirpath, fn), encoding="utf-8").read()
            for m in re.finditer(r"func (Test\w+)\(t \*testing\.T\) \{(.*?)\n\}", text, re.S):
                if "testhelper.Slow(" in m.group(2):
                    found.add(f"{os.path.relpath(dirpath, backend)}.{m.group(1)}")
    for name in sorted(found - set(listed)):
        failures.append(f"{name} calls testhelper.Slow but is not under slow_tests in test-budget.json (with its reason)")
    for name, reason in listed.items():
        if not reason.strip():
            failures.append(f"slow_tests {name} has no reason")
    if not failures:
        print(f"slowlist: {len(found)} slow tests, all listed with a reason")


def seconds(v):
    v = str(v).replace("_", "")
    if v[-1:] in "smh" and v[:-1].isdigit():
        return int(v[:-1]) * {"s": 1, "m": 60, "h": 3600}[v[-1]]
    return int(v) if v.isdigit() else 0


def cmd_diff(args):
    ref = args[0] if args else "origin/main"
    try:
        base = json.loads(subprocess.check_output(["git", "-C", ROOT, "show", f"{ref}:test-budget.json"], stderr=subprocess.DEVNULL))
    except subprocess.CalledProcessError:
        print(f"diff: no test-budget.json on {ref}, nothing to compare")
        return
    new = load()
    old_r, new_r = base.get("reasons", {}), new.get("reasons", {})

    def need(key, what):
        if not new_r.get(key, "").strip() or new_r.get(key) == old_r.get(key):
            failures.append(f"{what}: raised without a new reason under reasons[\"{key}\"] (why it is slow, why it can't be faster)")

    for mach, sections in new.get("machines", {}).items():
        old_s = base.get("machines", {}).get(mach, {})
        for sec, val in sections.items():
            entries = val.items() if sec == "go" else [(sec, val)]
            old_entries = old_s.get(sec, {}) if sec == "go" else {sec: old_s.get(sec)}
            for name, e in entries:
                key = "cpu_s" if "cpu_s" in e else "wall_s"
                o = old_entries.get(name)
                rkey = f"{sec}:{name}" if sec == "go" else sec
                if o is None:
                    if e[key] > NEW_ENTRY_FREE:
                        need(rkey, f"{mach} {sec} {name} is new at {e[key]}s")
                    continue
                n0 = max(o.get("tests", 0), 1)
                prop = o[key] + max(0, e.get("tests", 0) - o.get("tests", 0)) * o[key] / n0
                if e[key] > prop + 0.05:
                    need(rkey, f"{mach} {sec} {name} budget {o[key]} -> {e[key]}s (proportional: {prop:.1f}s)")
    for k in ("tolerance", "single_test_max_s"):
        if json.dumps(new.get(k), sort_keys=True) != json.dumps(base.get(k), sort_keys=True):
            if new.get(k) != base.get(k):
                need("policy", f"{k} changed")
    for rel, vals in new.get("timeouts", {}).items():
        old = base.get("timeouts", {}).get(rel, [])
        for i, v in enumerate(vals):
            if i >= len(old) or seconds(v) > seconds(old[i]):
                need(f"timeouts:{rel}", f"timeout {rel} #{i + 1} {old[i] if i < len(old) else 'new'} -> {v}")
    for name, reason in new.get("slow_tests", {}).items():
        if name not in base.get("slow_tests", {}) and not reason.strip():
            failures.append(f"slow_tests {name} is new without a reason")
    if not failures:
        print(f"diff: no budget or timeout raised against {ref} without a reason")


def main():
    cmd, args = sys.argv[1], sys.argv[2:]
    {
        "go": cmd_go,
        "race": cmd_race,
        "vitest": lambda a: cmd_wall("vitest", a, parse_vitest),
        "e2e": lambda a: cmd_wall("e2e", a, parse_e2e),
        "timeouts": cmd_timeouts,
        "slowlist": cmd_slowlist,
        "diff": cmd_diff,
    }[cmd](args)
    for n in notes:
        print("note:", n)
    for f in failures:
        print("BUDGET FAIL:", f, file=sys.stderr)
    sys.exit(1 if failures else 0)


if __name__ == "__main__":
    main()
