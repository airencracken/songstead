#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Release, service and license contracts with deliberately damaged inputs."""
from pathlib import Path
import subprocess
import unittest
ROOT=Path(__file__).resolve().parents[1]

def service_errors(text):
    return [requirement for requirement in ("User=songstead", "Group=songstead", "StateDirectoryMode=0700", "UMask=0077", "EnvironmentFile=-/etc/songstead/songstead.env", "ExecStart=/usr/local/bin/songstead serve") if requirement not in text]

class PackagingTests(unittest.TestCase):
    def test_generated_python_files_are_not_release_sources(self):
        tracked=subprocess.run(["git","ls-files","--cached"],cwd=ROOT,check=True,capture_output=True,text=True).stdout.splitlines()
        self.assertFalse([p for p in tracked if "__pycache__/" in p or p.endswith((".pyc",".pyo"))])
    def test_agpl_stack_and_version(self):
        self.assertIn("GNU AFFERO GENERAL PUBLIC LICENSE", (ROOT/"LICENSE").read_text())
        self.assertEqual((ROOT/"VERSION").read_text().strip(),"0.1.0")
        self.assertIn("modernc.org/sqlite",(ROOT/"go.mod").read_text())
        self.assertTrue((ROOT/"internal/web/static/htmx.min.js").is_file())
        make=(ROOT/"Makefile").read_text()
        self.assertIn('-X main.version=$(VERSION)',make)
    def test_service_is_unprivileged_and_configuration_is_loaded(self):
        text=(ROOT/"contrib/systemd/songstead.service").read_text()
        self.assertEqual(service_errors(text),[])
        for old in ("User=songstead","StateDirectoryMode=0700","EnvironmentFile=-/etc/songstead/songstead.env"):
            self.assertTrue(service_errors(text.replace(old,"")))
        subprocess.run(["bash","-n",str(ROOT/"contrib/openrc/songstead")],check=True)
        text=(ROOT/"contrib/openrc/songstead").read_text()
        for key in ("SONGSTEAD_BASE_URL","SONGSTEAD_WITMOOT_URL","SONGSTEAD_TRUSTED_PROXIES"):
            self.assertIn(key,text)
    def test_release_uses_published_dependencies_and_master(self):
        config=(ROOT/".goreleaser.yaml").read_text()
        for token in ("GOWORK=off","goarch: [amd64, arm64]","main.version=", "name: songstead", "THIRD_PARTY_NOTICES.txt"):
            self.assertIn(token,config)
        workflow=(ROOT/".github/workflows/release.yml").read_text()
        self.assertIn("HEAD origin/master",workflow)
        self.assertIn("git diff --exit-code -- go.mod go.sum",workflow)
        self.assertIn("make check build",workflow)
        prepare=(ROOT/"scripts/release/prepare.sh").read_text()
        self.assertIn("export GOWORK=off",prepare)
        self.assertIn("go mod verify || exit 1",prepare)
        subprocess.run(["sh","-n",str(ROOT/"scripts/release/prepare.sh")],check=True)

if __name__=="__main__":unittest.main(verbosity=2)
