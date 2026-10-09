#!/usr/bin/env python3
"""Sample keel-rex CPU and RSS on macOS/Linux, using only Python and ps."""

import argparse
import csv
from datetime import datetime
import math
import os
from pathlib import Path
import re
import subprocess
import sys
import time


def cpu_seconds(value):
    days, sep, clock = value.partition("-")
    seconds = 0.0
    for part in (clock if sep else value).split(":"):
        seconds = seconds * 60 + float(part)
    return seconds + (int(days) * 86400 if sep else 0)


def ps_output(*args):
    result = subprocess.run(
        ["ps", *args], capture_output=True, text=True, timeout=10,
        env={**os.environ, "LC_ALL": "C"},
    )
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or "ps failed")
    return result.stdout


def read_processes():
    processes = {}
    for line in ps_output("-axo", "pid=,ppid=,rss=,time=,comm=").splitlines():
        fields = line.split(None, 4)
        if len(fields) != 5:
            continue
        pid, parent, rss, cpu, executable = fields
        processes[int(pid)] = {
            "pid": int(pid), "parent": int(parent), "rss": int(rss) / 1024,
            "cpu_seconds": cpu_seconds(cpu), "executable": executable,
        }
    return processes


def is_rex(executable):
    name = Path(executable).name
    return name in {"Rex Keel", "Rex Keel Dev", "keel-rex", "rex-server"}


def select_processes(processes, pids, children):
    selected = set(pids) if pids else {
        pid for pid, process in processes.items() if is_rex(process["executable"])
    }
    roots = selected.copy()
    if children:
        while True:
            descendants = {
                pid for pid, process in processes.items()
                if process["parent"] in selected
            }
            expanded = selected | descendants
            if expanded == selected:
                break
            selected = expanded
    return [(processes[pid], pid in roots) for pid in sorted(selected) if pid in processes]


def role(process, root, commands):
    if not root:
        return "child"
    if Path(process["executable"]).name == "rex-server" or re.search(
        r"(?:^|\s)--?server(?:=true)?(?:\s|$)", commands.get(process["pid"], "")
    ):
        return "server"
    return "app"


def positive(value):
    number = float(value)
    if not math.isfinite(number) or number <= 0:
        raise argparse.ArgumentTypeError("must be a finite number greater than zero")
    return number


def positive_pid(value):
    number = int(value)
    if number <= 0:
        raise argparse.ArgumentTypeError("PID must be greater than zero")
    return number


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--interval", type=positive, default=1, help="sample interval in seconds (default: 1)")
    parser.add_argument("--duration", type=positive, help="stop after this many seconds")
    parser.add_argument("--pid", type=positive_pid, action="append", default=[], help="monitor a PID; repeat for multiple processes")
    parser.add_argument("--children", action="store_true", help="also include shells and their descendants")
    parser.add_argument("--csv", type=Path, help="write per-process and total samples to a new CSV file")
    args = parser.parse_args()
    if sys.platform not in {"darwin", "linux"}:
        parser.error("this monitor supports macOS and Linux")

    output = None
    writer = None
    previous = {}
    start = time.monotonic()
    peak_cpu = peak_rss = 0.0
    cpu_measured = False
    measured = 0
    try:
        if args.csv:
            output = args.csv.open("x", newline="", encoding="utf-8")
            writer = csv.writer(output)
            writer.writerow(["timestamp", "elapsed_s", "pid", "role", "cpu_percent", "rss_mib", "executable"])
        print("CPU: 100% = one core; memory: RSS MiB. Ctrl+C to stop.", flush=True)
        while True:
            processes = read_processes()
            selected = select_processes(processes, args.pid, args.children)
            # Read arguments separately: executable names can contain spaces, and
            # matching arbitrary command arguments would include the monitor itself.
            commands = {}
            if selected:
                for line in ps_output("-axo", "pid=,args=").splitlines():
                    fields = line.split(None, 1)
                    if len(fields) == 2:
                        commands[int(fields[0])] = fields[1]
            now = time.monotonic()
            elapsed = now - start
            stamp = datetime.now().astimezone().isoformat(timespec="seconds")
            rows = []
            current = {}
            for process, root in selected:
                pid = process["pid"]
                cpu = None
                old = previous.get(pid)
                identity = (process["executable"], process["parent"])
                if old and old[2] == identity and process["cpu_seconds"] >= old[0]:
                    cpu = 100 * (process["cpu_seconds"] - old[0]) / (now - old[1])
                current[pid] = (process["cpu_seconds"], now, identity)
                rows.append((pid, role(process, root, commands), cpu, process["rss"], process["executable"]))
            previous = current
            total_cpu = sum(row[2] for row in rows) if rows and all(row[2] is not None for row in rows) else None
            total_rss = sum(row[3] for row in rows)
            if rows:
                measured += 1
                peak_rss = max(peak_rss, total_rss)
                if total_cpu is not None:
                    cpu_measured = True
                    peak_cpu = max(peak_cpu, total_cpu)
                print(f"\n{stamp}  elapsed={elapsed:.1f}s")
                print(f"{'PID':>7}  {'ROLE':<6}  {'CPU %':>8}  {'RSS MiB':>9}  EXECUTABLE")
                for pid, kind, cpu, rss, executable in rows:
                    cpu_text = f"{cpu:.1f}" if cpu is not None else "--"
                    print(f"{pid:7}  {kind:<6}  {cpu_text:>8}  {rss:9.1f}  {executable}")
                cpu_text = f"{total_cpu:.1f}" if total_cpu is not None else "--"
                print(f"{'TOTAL':>7}  {'':6}  {cpu_text:>8}  {total_rss:9.1f}", flush=True)
            else:
                print(f"{stamp}  Waiting for {'PID(s) ' + ', '.join(map(str, args.pid)) if args.pid else 'keel-rex'}...", flush=True)
            if writer:
                for pid, kind, cpu, rss, executable in rows + [("", "total", total_cpu, total_rss, "")]:
                    writer.writerow([stamp, f"{elapsed:.3f}", pid, kind,
                                     f"{cpu:.3f}" if cpu is not None else "", f"{rss:.3f}", executable])
                output.flush()
            if args.duration and elapsed >= args.duration:
                break
            delay = max(0, args.interval - (time.monotonic() - now))
            if args.duration:
                delay = min(delay, max(0, start + args.duration - time.monotonic()))
            time.sleep(delay)
    except KeyboardInterrupt:
        print()
    except (OSError, RuntimeError, subprocess.TimeoutExpired) as error:
        print(f"monitor: {error}", file=sys.stderr)
        return 1
    finally:
        if output:
            output.close()
    if measured:
        cpu_text = f"{peak_cpu:.1f}%" if cpu_measured else "--"
        print(f"Peak sampled total: CPU {cpu_text}, RSS {peak_rss:.1f} MiB")
    return 0


if __name__ == "__main__":
    sys.exit(main())
