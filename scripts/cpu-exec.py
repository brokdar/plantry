#!/usr/bin/env python3
"""go test -exec wrapper: runs one test binary and records its CPU time.

    go test -exec "$PWD/../scripts/cpu-exec.py" ./...

Appends {"dir": <package dir>, "cpu_s": <user+sys>} to $GO_TEST_CPU_LOG.
go test runs each binary in its package directory, so the working directory
names the package. CPU time, unlike wall time, does not depend on how many
cores the machine has or what else runs, which makes it the budget unit of
scripts/test-budget.py.
"""
import json
import os
import resource
import subprocess
import sys

rc = subprocess.call(sys.argv[1:])
use = resource.getrusage(resource.RUSAGE_CHILDREN)
log = os.environ.get("GO_TEST_CPU_LOG")
if log:
    with open(log, "a", encoding="utf-8") as f:
        f.write(json.dumps({"dir": os.getcwd(), "cpu_s": round(use.ru_utime + use.ru_stime, 2)}) + "\n")
sys.exit(rc)
