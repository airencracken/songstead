#!/usr/bin/env python3
"""Require deliberate domain and security regressions to fail their named tests.

Each mutation runs in a disposable copy. A local Comfylib checkout is copied too
while the coordinated library release is pending; released builds use go.mod.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
TABLE = ROOT / "scripts/mutations.json"


def main():
    mutations = json.loads(TABLE.read_text())
    failures = 0
    for entry in mutations:
        source = (ROOT / entry["file"]).read_text()
        if source.count(entry["before"]) != 1:
            raise ValueError(f"Mutation anchor must occur exactly once: {entry['name']}")
        with tempfile.TemporaryDirectory(prefix="songstead-mutation-") as directory:
            base = Path(directory)
            checkout = base / "songstead"
            ignore = shutil.ignore_patterns(".git", "bin", "data", "design", "go.work", "go.work.sum", "__pycache__", ".artifacts")
            shutil.copytree(ROOT, checkout, ignore=ignore)
            env = dict(os.environ, GOPROXY="off", GOSUMDB="off", GOWORK="off")
            library = ROOT.parent / "comfylib"
            if (library / "go.mod").is_file() and (ROOT / "go.work").is_file():
                shutil.copytree(library, base / "comfylib", ignore=ignore)
                workspace = base / "go.work"
                workspace.write_text("go 1.26.0\nuse (\n ./songstead\n ./comfylib\n)\n"
                                     "replace github.com/airencracken/comfylib v0.1.1 => ./comfylib\n")
                env["GOWORK"] = str(workspace)
            command = ["go", "test", "-count=1", "-run", entry["run"], entry["package"]]
            baseline = subprocess.run(command, cwd=checkout, env=env, capture_output=True, text=True, timeout=120)
            if baseline.returncode or "no tests to run" in baseline.stdout:
                print("BASELINE FAILED:", entry["name"], baseline.stdout, baseline.stderr)
                failures += 1
                continue
            target = checkout / entry["file"]
            target.write_text(source.replace(entry["before"], entry["after"], 1))
            result = subprocess.run(command, cwd=checkout, env=env, capture_output=True, text=True, timeout=120)
            caught = result.returncode != 0 and re.search(r"--- FAIL:", result.stdout) is not None
            print(("PASS:" if caught else "SURVIVED OR BROKEN:"), entry["name"], flush=True)
            if not caught:
                print(result.stdout, result.stderr)
                failures += 1
    return int(failures != 0)


if __name__ == "__main__":
    raise SystemExit(main())
