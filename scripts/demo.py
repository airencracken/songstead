#!/usr/bin/env python3
"""Run an isolated, disposable two-friend demo without inherited configuration."""
import os
from pathlib import Path
import subprocess
import tempfile

root = Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix="songstead-demo-") as directory:
    env = {k: v for k, v in os.environ.items() if not k.startswith("SONGSTEAD_")}
    binary = root / "bin/songstead"
    for username in ("alice", "bobby"):
        subprocess.run([binary, "create-user", "--data-dir", directory, "--username", username,
                        "--password-stdin"], input="demo-password\n", text=True, env=env, check=True)
    print("Demo: http://127.0.0.1:8083. Accounts: alice, bobby. Password: demo-password.", flush=True)
    process = subprocess.Popen([binary, "serve", "--addr", "127.0.0.1:8083", "--data-dir", directory], env=env)
    try:
        process.wait()
    except KeyboardInterrupt:
        process.terminate()
        process.wait(timeout=15)
