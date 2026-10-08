#!/usr/bin/env python3
"""Exercise the real release shell with command doubles, never live services."""

import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = "ppcelery/go-ramjet:latest"
HEALTH_URL = "http://172.20.0.4:24456/gptchat/version"
SECRET_MARKER = "synthetic-deployment-test-marker"

FAKE_COMMAND = r"""#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

args = sys.argv[1:]
binary = Path(sys.argv[0]).name
event = {"binary": binary, "args": args,
         "secret_inherited": os.environ.get("RAMJET_TEST_SECRET") == "synthetic-deployment-test-marker"}
log = Path(os.environ["COMMAND_LOG"])
history = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
with log.open("a") as output:
    output.write(json.dumps(event) + "\n")
failure = os.environ.get("FAILURE", "")
if binary == "docker-compose":
    sys.exit(42)
if binary == "curl":
    if failure == "health":
        sys.exit(22)
    print("503" if failure == "health-status" else "200", end="")
elif args == ["compose", "version", "--short"]:
    if failure == "version":
        sys.exit(41)
    print(os.environ.get("COMPOSE_VERSION", "2.39.4"))
elif args[0] == "compose" and "config" in args:
    if failure == "config":
        sys.exit(43)
elif args[0] == "compose" and "ps" in args:
    if failure != "missing-container":
        changed = any("up" in previous["args"] for previous in history)
        print("new-container" if changed else "old-container")
elif args[0] == "compose" and "up" in args:
    if failure == "up":
        sys.exit(45)
elif args[0] == "pull":
    if failure == "pull":
        sys.exit(44)
elif args[:2] == ["image", "inspect"]:
    if failure == "image-inspect":
        sys.exit(46)
    print("sha256:" + "2" * 64)
elif args[0] == "inspect":
    if any(".IPAddress" in arg for arg in args):
        if failure != "missing-ip":
            print("172.20.0.4")
    elif "{{.State.Running}}" in args:
        print("false" if failure == "stopped" else "true")
    else:
        print("sha256:" + ("1" if args[-1] == "old-container" else "3" if failure == "mismatch" else "2") * 64)
elif args == ["ps"]:
    print("final informational command succeeds")
else:
    print("Unexpected mocked command: " + repr(args), file=sys.stderr)
    sys.exit(99)
"""


def extract_release_script(workflow):
    """extract_release_script reads the workflow Path and returns its unique literal SSH script."""
    lines = workflow.read_text().splitlines()
    starts = [index for index, line in enumerate(lines) if line.strip() == "script: |"]
    if len(starts) != 1:
        raise ValueError("expected exactly one remote deployment script")
    start = starts[0]
    indent = len(lines[start]) - len(lines[start].lstrip()) + 2
    script = []
    for line in lines[start + 1:]:
        if line.strip() and len(line) - len(line.lstrip()) < indent:
            break
        script.append(line[indent:])
    return "\n".join(script) + "\n"


class ReleaseDeploymentContract(unittest.TestCase):
    """ReleaseDeploymentContract validates release commands against isolated command doubles."""

    def setUp(self):
        """setUp creates command doubles and isolated files for each test and returns no value."""
        self.temporary = tempfile.TemporaryDirectory(prefix="ramjet-release-contract-")
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)
        self.log = self.directory / "commands.jsonl"
        for binary in ("docker", "docker-compose", "curl"):
            executable = self.directory / binary
            executable.write_text(FAKE_COMMAND)
            executable.chmod(0o755)
        (self.directory / "b1-docker-compose.yml").write_text("services: {}\n")

    def run_deployment(self, failure="", version="2.39.4"):
        """run_deployment executes the real script with failure/version doubles and returns its result."""
        self.log.unlink(missing_ok=True)
        script = extract_release_script(ROOT / ".github/workflows/ci.yml")
        script = script.replace("${{ github.sha }}", "1234567890abcdef1234567890abcdef12345678")
        script = script.replace("/home/laisky/repo/VPS", shlex.quote(str(self.directory)))
        env = {**os.environ, "PATH": str(self.directory) + os.pathsep + os.environ["PATH"],
               "COMMAND_LOG": str(self.log), "FAILURE": failure, "COMPOSE_VERSION": version,
               "RAMJET_TEST_SECRET": SECRET_MARKER}
        return subprocess.run(["bash"], input=script, text=True, capture_output=True, env=env, timeout=10)

    def events(self):
        """events returns the recorded command events from the current test."""
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_uses_only_modern_compose_with_version_and_config_preflight(self):
        """test_uses_only_modern_compose_with_version_and_config_preflight verifies CLI safety and returns no value."""
        result = self.run_deployment()
        self.assertEqual(result.returncode, 0, result.stderr)
        events = self.events()
        self.assertTrue(all(event["binary"] in ("docker", "curl") for event in events))
        self.assertEqual(events[0]["args"], ["compose", "version", "--short"])
        self.assertEqual(events[1]["args"], ["compose", "-f", "b1-docker-compose.yml", "config", "--quiet"])

    def test_pulls_latest(self):
        """test_pulls_latest checks the user's existing mutable release tag and returns no value."""
        result = self.run_deployment()
        self.assertEqual(result.returncode, 0, result.stderr)
        pulls = [event["args"] for event in self.events() if event["args"][0] == "pull"]
        self.assertEqual(pulls, [["pull", IMAGE]])

    def test_recreates_only_ramjet_without_dependencies_or_orphan_removal(self):
        """test_recreates_only_ramjet_without_dependencies_or_orphan_removal checks service scope and returns no value."""
        result = self.run_deployment()
        self.assertEqual(result.returncode, 0, result.stderr)
        events = self.events()
        up = [event for event in events if "up" in event["args"]]
        self.assertEqual(len(up), 1)
        self.assertEqual(up[0]["binary"], "docker")
        self.assertEqual(up[0]["args"], ["compose", "-f", "b1-docker-compose.yml", "up", "-d",
                                       "--no-deps", "--force-recreate", "go-ramjet"])
        self.assertFalse(any("--remove-orphans" in event["args"] or "down" in event["args"]
                             or "prune" in event["args"] for event in events))

    def test_reuses_existing_configuration_without_overrides_or_pinning(self):
        """test_reuses_existing_configuration_without_overrides_or_pinning preserves the operator's Compose definition."""
        result = self.run_deployment()
        self.assertEqual(result.returncode, 0, result.stderr)
        for event in self.events():
            args = event["args"]
            if args[0] == "compose" and args[1] != "version":
                files = [args[index + 1] for index, value in enumerate(args) if value == "-f"]
                self.assertEqual(files, ["b1-docker-compose.yml"])
            self.assertNotIn("old-container", args)
            self.assertFalse(any("@sha256:" in arg for arg in args))

    def test_inherits_environment_without_printing_secret_values(self):
        """test_inherits_environment_without_printing_secret_values checks injected secret inheritance and output masking."""
        result = self.run_deployment()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(all(event["secret_inherited"] for event in self.events()))
        self.assertNotIn(SECRET_MARKER, result.stdout + result.stderr + self.log.read_text())

    def test_verifies_running_service_and_version_health(self):
        """test_verifies_running_service_and_version_health checks a scoped container-IP probe and explicit HTTP 200."""
        result = self.run_deployment()
        self.assertEqual(result.returncode, 0, result.stderr)
        args = [event["args"] for event in self.events()]
        self.assertIn(["inspect", "--format", "{{.State.Running}}", "new-container"], args)
        self.assertIn(["image", "inspect", "--format", "{{.Id}}", IMAGE], args)
        self.assertIn(["inspect", "--format", "{{.Image}}", "new-container"], args)
        health = [event for event in self.events() if event["binary"] == "curl"]
        self.assertEqual(len(health), 1)
        self.assertEqual(health[0]["args"][-1], HEALTH_URL)
        self.assertIn("--fail", health[0]["args"])
        self.assertIn("--output", health[0]["args"])
        self.assertIn("/dev/null", health[0]["args"])

    def test_version_config_and_pull_failures_stop_before_container_changes(self):
        """test_version_config_and_pull_failures_stop_before_container_changes checks fail-fast preflight failures."""
        for failure in ("version", "config", "pull", "image-inspect"):
            with self.subTest(failure=failure):
                result = self.run_deployment(failure=failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any("up" in event["args"] for event in self.events()))
                if failure in ("version", "config"):
                    self.assertFalse(any("pull" in event["args"] for event in self.events()))

    def test_accepts_supported_modern_compose_versions(self):
        """test_accepts_supported_modern_compose_versions permits installed plugin major versions of at least two."""
        for version in ("v2.39.4", "5.1.3"):
            with self.subTest(version=version):
                result = self.run_deployment(version=version)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_rejects_compose_v1_before_pull_or_up(self):
        """test_rejects_compose_v1_before_pull_or_up verifies explicit v2 compatibility and returns no value."""
        result = self.run_deployment(version="1.28.4")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any("pull" in event["args"] or "up" in event["args"] for event in self.events()))

    def test_up_failure_cannot_be_masked_by_later_commands(self):
        """test_up_failure_cannot_be_masked_by_later_commands checks fail-fast deployment and returns no value."""
        result = self.run_deployment(failure="up")
        self.assertNotEqual(result.returncode, 0)
        events = self.events()
        self.assertEqual(events[-1]["args"][-1], "go-ramjet")
        self.assertIn("up", events[-1]["args"])

    def test_missing_stopped_or_unhealthy_service_fails_deployment(self):
        """test_missing_stopped_or_unhealthy_service_fails_deployment rejects missing containers, addresses, or health."""
        for failure in ("missing-container", "stopped", "mismatch", "missing-ip", "health", "health-status"):
            with self.subTest(failure=failure):
                result = self.run_deployment(failure=failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(len([event for event in self.events() if "up" in event["args"]]), 1)
                if failure in ("missing-container", "stopped", "mismatch", "missing-ip"):
                    self.assertFalse(any(event["binary"] == "curl" for event in self.events()))

    def test_pr_validation_runs_contract_before_go_unit_tests(self):
        """test_pr_validation_runs_contract_before_go_unit_tests checks the required workflow invocation."""
        workflow = (ROOT / ".github/workflows/pr-validation.yml").read_text()
        command = "python3 ci/test_release_deployment.py"
        self.assertIn(command, workflow)
        self.assertLess(workflow.index(command), workflow.index("run: go test ./... -count=1"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
