#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Exercise hidden password prompts through an actual terminal and binary."""
import errno
import os
from pathlib import Path
import pty
import select
import sqlite3
import subprocess
import tempfile
import termios
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "bin/songstead"


class TerminalPasswordTests(unittest.TestCase):
    def provision(self, data, first, second):
        master, slave = pty.openpty()
        before = termios.tcgetattr(slave)
        process = subprocess.Popen(
            [str(BINARY), "create-owner", "--data-dir", str(data),
             "--username", "alex", "--password-prompt"],
            stdin=slave, stdout=slave, stderr=slave,
        )
        transcript = bytearray()
        deadline = time.monotonic() + 10

        def until(marker):
            while marker not in transcript:
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    self.fail("terminal prompt timed out")
                if select.select([master], [], [], remaining)[0]:
                    chunk = os.read(master, 4096)
                    if not chunk:
                        self.fail("terminal closed before prompt")
                    transcript.extend(chunk)

        try:
            until(b"Password: ")
            os.write(master, first.encode() + b"\n")
            until(b"Confirm password: ")
            os.write(master, second.encode() + b"\n")
            code = process.wait(timeout=10)
            while select.select([master], [], [], 0)[0]:
                try:
                    chunk = os.read(master, 4096)
                except OSError as error:
                    if error.errno != errno.EIO:
                        raise
                    break
                if not chunk:
                    break
                transcript.extend(chunk)
            self.assertEqual(termios.tcgetattr(slave), before,
                             "password prompt did not restore terminal settings")
            self.assertNotIn(first.encode(), transcript, "first password echoed")
            self.assertNotIn(second.encode(), transcript, "confirmation echoed")
            return code, bytes(transcript)
        finally:
            if process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            os.close(master)
            os.close(slave)

    def test_hidden_prompt_creates_owner(self):
        with tempfile.TemporaryDirectory(prefix="songstead-cli-") as directory:
            data = Path(directory) / "data"
            code, transcript = self.provision(data, "a-long-owner-password", "a-long-owner-password")
            self.assertEqual(code, 0, transcript.decode(errors="replace"))
            with sqlite3.connect(data / "songstead.db") as database:
                self.assertEqual(database.execute("SELECT username,role FROM users").fetchall(),
                                 [("alex", "owner")])
            self.assertEqual((data / "songstead.db").stat().st_mode & 0o777, 0o600)

    def test_mismatched_prompt_leaves_no_database(self):
        with tempfile.TemporaryDirectory(prefix="songstead-cli-") as directory:
            data = Path(directory) / "data"
            code, transcript = self.provision(data, "first-long-password", "second-long-password")
            self.assertNotEqual(code, 0)
            self.assertIn(b"passwords do not match", transcript)
            self.assertFalse(data.exists())

    def test_pipe_requires_password_stdin(self):
        with tempfile.TemporaryDirectory(prefix="songstead-cli-") as directory:
            data = Path(directory) / "data"
            result = subprocess.run(
                [str(BINARY), "create-owner", "--data-dir", str(data),
                 "--username", "alex", "--password-prompt"],
                input="a-long-owner-password\n", text=True, capture_output=True, timeout=10,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("requires a terminal", result.stderr)
            self.assertFalse(data.exists())


if __name__ == "__main__":
    unittest.main()
