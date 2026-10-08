#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Collect license texts from the modules linked into the released binaries."""
import os
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[2]

def notices():
    modules = {}
    for arch in ("amd64", "arm64"):
        result = subprocess.run(["go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}|{{.Dir}}{{end}}", "./cmd/songstead"],
            cwd=ROOT, env={**os.environ, "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": arch}, check=True, capture_output=True, text=True)
        for line in result.stdout.splitlines():
            if "|" in line:
                name, directory = line.split("|", 1)
                if name != "github.com/airencracken/songstead":
                    modules[name] = Path(directory)
    goroot = subprocess.run(["go", "env", "GOROOT"], check=True, capture_output=True, text=True).stdout.strip()
    output = ["Go runtime\n", (ROOT / "contrib/licenses/go.LICENSE").read_text()]
    for name, directory in sorted(modules.items()):
        files = sorted({p for pattern in ("LICENSE*", "COPYING*", "NOTICE*") for p in directory.glob(pattern) if p.is_file()})
        if not files:
            raise RuntimeError(f"Missing license notice for {name}")
        for path in files:
            output.extend([f"\n--- {name}: {path.name} ---\n", path.read_text()])
    output.extend(["\n--- HTMX ---\n", (ROOT / "internal/web/static/htmx.LICENSE").read_text()])
    return "\n".join(output)

if __name__ == "__main__":
    print(notices())
