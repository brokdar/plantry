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

# CI's runner has 4 vCPUs and is the reference machine. On a many-core machine
# under load every test binary otherwise starts one scheduler thread per core;
# the idle ones spin and the CPU seconds grow with the machine's load.
env = dict(os.environ)
env.setdefault("GOMAXPROCS", "4")
rc = subprocess.call(sys.argv[1:], env=env)
use = resource.getrusage(resource.RUSAGE_CHILDREN)
cpu = round(use.ru_utime + use.ru_stime, 2)
# CPU seconds depend on the machine's speed at that moment (hyperthreads shared
# with other lanes' work run each instruction up to 2x slower). With CALIB_BIN
# set (scripts/calibrate.go, built once by the gate), a binary that used over a
# second also records the speed right after its run, under the same load, and
# scripts/test-budget.py scales that package's CPU by it.
calib = None
calib_bin = os.environ.get("CALIB_BIN")
if calib_bin and cpu > 1.0:
    try:
        calib = float(subprocess.check_output([calib_bin, "quick"], text=True))
    except (OSError, ValueError, subprocess.CalledProcessError):
        calib = None
log = os.environ.get("GO_TEST_CPU_LOG")
if log:
    with open(log, "a", encoding="utf-8") as f:
        rec = {"dir": os.getcwd(), "cpu_s": cpu}
        if calib:
            rec["calib_s"] = calib
        f.write(json.dumps(rec) + "\n")
sys.exit(rc)
