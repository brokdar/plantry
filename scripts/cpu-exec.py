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
import subprocess
import sys
import time

# CI's runner has 4 vCPUs and is the reference machine. On a many-core machine
# under load every test binary otherwise starts one scheduler thread per core;
# the idle ones spin and the CPU seconds grow with the machine's load.
env = dict(os.environ)
env.setdefault("GOMAXPROCS", "4")
# CPU seconds depend on the machine's speed while the binary ran (hyperthreads
# shared with other lanes' work run each instruction up to 2x slower). With
# CALIB_BIN set (scripts/calibrate.go, built once by the gate) the speed is
# probed DURING the run, under the load the tests ran in (one probe of 0.5
# CPU-s every PROBE_EVERY seconds from PROBE_FROM on; the best one counts, as
# the baseline is a best-of-3), and scripts/test-budget.py scales that package
# by it. A probe after the run would see the machine quieter than it was. A run
# that ends before the first probe is probed once afterwards if it used over a
# second of CPU.
PROBE_FROM, PROBE_EVERY = 4.0, 8.0
calib_bin = os.environ.get("CALIB_BIN")


def probe():
    try:
        return float(subprocess.check_output([calib_bin, "1"], text=True))
    except (OSError, ValueError, subprocess.CalledProcessError):
        return None


proc = subprocess.Popen(sys.argv[1:], env=env)
# wait4 on the test binary alone: its rusage does not include the probes.
start, next_probe, samples = time.monotonic(), PROBE_FROM, []
while True:
    pid, status, use = os.wait4(proc.pid, os.WNOHANG)
    if pid:
        break
    if calib_bin and time.monotonic() - start >= next_probe:
        v = probe()
        if v:
            samples.append(v)
        next_probe += PROBE_EVERY
    else:
        time.sleep(0.1)
rc = os.waitstatus_to_exitcode(status)
cpu = round(use.ru_utime + use.ru_stime, 2)
calib = min(samples) if samples else None
if calib_bin and calib is None and cpu > 1.0:
    calib = probe()
log = os.environ.get("GO_TEST_CPU_LOG")
if log:
    with open(log, "a", encoding="utf-8") as f:
        rec = {"dir": os.getcwd(), "cpu_s": cpu}
        if calib:
            rec["calib_s"] = calib
        f.write(json.dumps(rec) + "\n")
sys.exit(rc)
