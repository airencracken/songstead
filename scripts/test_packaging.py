#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
"""Release, service and license contracts with deliberately damaged inputs."""
from pathlib import Path
import os
import re
from urllib.parse import urlsplit
import subprocess
import tempfile
import unittest
ROOT=Path(__file__).resolve().parents[1]

def example_configuration_errors(text):
    errors = []
    for raw in re.findall(r'SONGSTEAD_[A-Z_]+=["\']?(https?://[^\s"\']+)', text):
        try:
            url = urlsplit(raw)
            host = url.hostname or ""
            allowed = host in ("localhost", "127.0.0.1", "::1") or any(
                host == domain or host.endswith("." + domain)
                for domain in ("example.com", "example.org", "example.net"))
            if not allowed or url.username is not None or url.password is not None:
                errors.append("Deployment examples must use credential-free example or loopback hosts")
        except ValueError:
            errors.append("Malformed example address")
    return errors

def service_errors(text):
    return [requirement for requirement in ("User=songstead", "Group=songstead", "StateDirectoryMode=0700", "UMask=0077", "EnvironmentFile=-/etc/songstead/songstead.env", "ExecStart=/usr/local/bin/songstead serve") if requirement not in text]

class PackagingTests(unittest.TestCase):
    def test_funding_matches_the_companion_projects(self):
        self.assertEqual((ROOT / ".github/FUNDING.yml").read_text().strip(),
                         "ko_fi: airencracken")
        self.assertIn("## Support\n\nYou can [support Songstead on Ko-fi](https://ko-fi.com/airencracken).",
                      (ROOT / "README.md").read_text())
    def run_openrc_start(self, overrides=None):
        script = '''
eerror() { printf '%s\\n' "$*" >&2; }
checkpath() {
    printf 'checkpath'; printf ' <%s>' "$@"; printf '\\n'
    [ "${FAIL_CHECKPATH:-}" != yes ]
}
. "$1" || exit 1
start_pre || exit 1
printf 'command <%s> args <%s> user <%s> umask <%s>\\n' "$command" "$command_args" "$command_user" "$(umask)"
'''
        settings = {"PATH": os.environ["PATH"], "RC_SVCNAME": "songstead",
                    "SONGSTEAD_DATA_DIR": "/var/lib/songstead",
                    "SONGSTEAD_BIN": "/usr/local/bin/songstead",
                    "SONGSTEAD_LOG_FILE": "/tmp/songstead.log", **(overrides or {})}
        return subprocess.run(["sh", "-c", script, "openrc-test",
                               str(ROOT / "contrib/openrc/songstead")],
                              env=settings, capture_output=True, text=True, timeout=10)

    def test_openrc_initializes_private_paths_and_serve_command(self):
        result = self.run_openrc_start()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("<--directory> <--mode> <0700> <--owner> <songstead:songstead> </var/lib/songstead>", result.stdout)
        self.assertIn("<--file> <--mode> <0640> <--owner> <songstead:songstead> </tmp/songstead.log>", result.stdout)
        self.assertIn("command </usr/local/bin/songstead> args <serve> user <songstead:songstead> umask <0077>", result.stdout)

    def test_openrc_sandbox_selection_is_explicit_and_validated(self):
        result = self.run_openrc_start({"SONGSTEAD_SANDBOX": "true"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("args <sandbox>", result.stdout)
        for value in ("yes", "1", "TRUE", "true; touch /tmp/injected"):
            result = self.run_openrc_start({"SONGSTEAD_SANDBOX": value})
            self.assertNotEqual(result.returncode, 0)
            self.assertNotIn("checkpath", result.stdout)
            self.assertIn("must be true or false", result.stderr)

    def test_systemd_sandbox_dropin_and_native_ci_checks(self):
        dropin = (ROOT / "contrib/systemd/songstead-sandbox.conf").read_text()
        self.assertIn("ExecStart=\nExecStart=/usr/local/bin/songstead sandbox", dropin)
        self.assertIn("TimeoutStopSec=25", dropin)
        self.assertIn("RestrictNamespaces=user pid ipc uts mnt", dropin)
        for name in ("ci.yml", "release.yml"):
            workflow = (ROOT / ".github/workflows" / name).read_text()
            self.assertIn("install -y bubblewrap", workflow)
            self.assertIn("make check test-sandbox build", workflow)

    def test_openrc_rejects_relative_and_adversarial_paths_before_changes(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "injected"
            for setting in ("SONGSTEAD_DATA_DIR", "SONGSTEAD_BIN", "SONGSTEAD_LOG_FILE"):
                for value in ("relative/path", "", f"$(touch {marker})", f".; touch {marker}"):
                    with self.subTest(setting=setting, value=value):
                        # Empty settings deliberately use the service defaults.
                        if not value:
                            self.assertEqual(self.run_openrc_start({setting: value}).returncode, 0)
                            continue
                        result = self.run_openrc_start({setting: value})
                        self.assertNotEqual(result.returncode, 0)
                        self.assertIn("Songstead paths must be absolute:", result.stderr)
                        self.assertNotIn("checkpath", result.stdout)
                        self.assertFalse(marker.exists())

    def test_openrc_stops_after_failed_data_directory_setup(self):
        result = self.run_openrc_start({"FAIL_CHECKPATH": "yes"})
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout.count("checkpath"), 1)
        self.assertNotIn("<--file>", result.stdout)
        self.assertNotIn("command <", result.stdout)

    def test_native_service_documentation_uses_generic_example_hosts(self):
        guide = (ROOT / "docs/deployment.md").read_text()
        self.assertEqual(example_configuration_errors(guide), [])
        service = (ROOT / "contrib/openrc/songstead").read_text()
        for key in ("SONGSTEAD_BASE_URL", "SONGSTEAD_WITMOOT_URL",
                    "SONGSTEAD_SECURE_COOKIES", "SONGSTEAD_TRUSTED_PROXIES"):
            self.assertIn(key, guide)
            self.assertIn(key, service)
        for old, new in (("songstead.example.com", "private-host.test"),
                         ("boards.example.com", "user:password@boards.example.com")):
            mutated = guide.replace(old, new)
            self.assertNotEqual(mutated, guide)
            self.assertTrue(example_configuration_errors(mutated))

    def test_example_configuration_validation(self):
        for host in ("example.com", "songstead.example.org", "boards.example.net",
                     "localhost", "127.0.0.1", "[::1]"):
            with self.subTest(host=host):
                self.assertEqual(example_configuration_errors(
                    f'SONGSTEAD_BASE_URL="https://{host}"'), [])
        for host in ("example.com.attacker.test", "notexample.com", "private-host.test",
                     "192.0.2.55", "user@example.com", "[broken"):
            with self.subTest(host=host):
                self.assertTrue(example_configuration_errors(
                    f'SONGSTEAD_BASE_URL="https://{host}"'))

    def test_local_documentation_links_exist(self):
        for path in [ROOT / "README.md", *(ROOT / "docs").glob("*.md")]:
            for target in re.findall(r'\[[^\]\n]*\]\(([^)\s]+)\)', path.read_text()):
                if urlsplit(target).scheme or target.startswith("#"):
                    continue
                with self.subTest(path=path.name, target=target):
                    self.assertTrue((path.parent / target.split("#", 1)[0]).exists())

    def test_generated_python_files_are_not_release_sources(self):
        tracked=subprocess.run(["git","ls-files","--cached"],cwd=ROOT,check=True,capture_output=True,text=True).stdout.splitlines()
        self.assertFalse([p for p in tracked if "__pycache__/" in p or p.endswith((".pyc",".pyo"))])
    def test_agpl_stack_and_version(self):
        self.assertIn("GNU AFFERO GENERAL PUBLIC LICENSE", (ROOT/"LICENSE").read_text())
        self.assertEqual((ROOT/"VERSION").read_text().strip(),"0.1.2")
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
        self.assertIn("make check test-sandbox build",workflow)
        prepare=(ROOT/"scripts/release/prepare.sh").read_text()
        self.assertIn("export GOWORK=off",prepare)
        self.assertIn("go mod verify || exit 1",prepare)
        subprocess.run(["sh","-n",str(ROOT/"scripts/release/prepare.sh")],check=True)

    def test_release_guides_and_library_pin_are_self_contained(self):
        guide = (ROOT / "docs/releases.md").read_text()
        for requirement in ("songstead_0.1.2_linux_amd64.tar.gz", "songstead_0.1.2_checksums.txt",
                            "sha256sum --check --ignore-missing", "GOWORK=off make check build",
                            "=www-apps/songstead-0.1.2::comfyware"):
            self.assertIn(requirement, guide)
        module = (ROOT / "go.mod").read_text()
        self.assertIn("github.com/airencracken/comfylib v0.1.1", module)
        self.assertNotIn("replace ", module)
        sums = (ROOT / "go.sum").read_text()
        self.assertRegex(sums, r"(?m)^github.com/airencracken/comfylib v0\.1\.1 h1:[A-Za-z0-9+/]{43}=$")
        self.assertRegex(sums, r"(?m)^github.com/airencracken/comfylib v0\.1\.1/go\.mod h1:[A-Za-z0-9+/]{43}=$")

    def test_ci_checks_standalone_dependencies_without_rewriting_checksums(self):
        workflow = (ROOT / ".github/workflows/ci.yml").read_text()
        for requirement in ("GOWORK: 'off'", "shell: bash --noprofile --norc {0}",
                            "go mod download || exit 1", "go mod verify || exit 1",
                            "git diff --exit-code -- go.mod go.sum || exit 1",
                            "make check test-sandbox build"):
            self.assertIn(requirement, workflow)

if __name__=="__main__":unittest.main(verbosity=2)
