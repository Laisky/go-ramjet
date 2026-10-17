#!/usr/bin/env python3
"""Exercise the real release shell with command doubles, never live services."""

import json
import os
import re
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

from browser_sentry_build_args import (
    INVALID_MESSAGE, OUTPUT_DELIMITER, PublicConfigurationError, prepare_browser_build_args,
)

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


def release_job_decisions(workflow, message):
    """release_job_decisions executes the real prefix check and evaluates supported job selectors."""
    source = workflow.read_text()

    def job_body(name):
        """job_body returns the named job's source without subsequent jobs."""
        body = source.split("  " + name + ":\n", 1)[1]
        return re.split(r"^  [a-zA-Z_][a-zA-Z_0-9-]*:\n", body, maxsplit=1, flags=re.MULTILINE)[0]

    check = job_body("check_skip")
    lines = check.splitlines()
    starts = [index for index, line in enumerate(lines) if line.strip() == "run: |"]
    if len(starts) != 1:
        raise AssertionError("Expected exactly one commit-prefix check script")
    start = starts[0]
    indent = len(lines[start]) - len(lines[start].lstrip()) + 2
    script = []
    for line in lines[start + 1:]:
        if line.strip() and len(line) - len(line.lstrip()) < indent:
            break
        script.append(line[indent:])
    with tempfile.TemporaryDirectory(prefix="release-selector-contract-") as directory:
        output = Path(directory) / "outputs"
        result = subprocess.run(
            ["bash"], input="\n".join(script) + "\n", text=True, capture_output=True, timeout=5,
            env={"PATH": os.environ["PATH"], "COMMIT_MSG": message, "GITHUB_OUTPUT": str(output)},
        )
        if result.returncode:
            raise AssertionError(result.stderr)
        outputs = dict(line.split("=", 1) for line in output.read_text().splitlines())
    skip = outputs["should_skip"]

    def selected(name):
        """selected evaluates the workflow's restricted release expression grammar."""
        matches = re.findall(r"^    if: (.+)$", job_body(name), flags=re.MULTILINE)
        if len(matches) != 1:
            raise AssertionError("Expected exactly one selector for " + name)
        decisions = []
        for predicate in matches[0].split(" && "):
            if predicate == "needs.check_skip.outputs.should_skip != 'true'":
                decisions.append(skip.casefold() != "true")
                continue
            marker = re.fullmatch(r"!contains\(github\.event\.head_commit\.message, '([^']*)'\)", predicate)
            if marker is None:
                raise AssertionError("Unsupported release selector: " + predicate)
            # GitHub contains() is case insensitive; see its expressions reference.
            decisions.append(marker.group(1).casefold() not in message.casefold())
        return all(decisions)

    return tuple(selected(name) for name in ("build_latest", "build_latest_cache", "deploy"))


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
        command = "python3 -B ci/test_release_deployment.py"
        self.assertIn(command, workflow)
        self.assertNotIn("run: python3 ci/test_release_deployment.py", workflow)
        self.assertLess(workflow.index(command), workflow.index("run: go test ./... -count=1"))

    def test_helper_only_changes_run_unfiltered_pr_contract_validation(self):
        """test_helper_only_changes_run_unfiltered_pr_contract_validation checks helper coverage without release-trigger changes."""
        workflow = (ROOT / ".github/workflows/pr-validation.yml").read_text()
        triggers = workflow.split("permissions:", 1)[0]
        self.assertIn("pull_request:", triggers)
        self.assertIn("      - master", triggers)
        self.assertNotRegex(triggers, r"(?m)^\s+paths(?:-ignore)?:")
        self.assertIn("run: python3 -B ci/test_release_deployment.py", workflow)

    def test_release_selectors_preserve_builds_and_gate_only_marked_deployment(self):
        """test_release_selectors_preserve_builds_and_gate_only_marked_deployment executes actual selectors."""
        cases = [
            ("fix: ordinary release", (True, True, True)),
            ("fix: docs wording without a skipped prefix", (True, True, True)),
            ("fix: quoted '$()' commit text", (True, True, True)),
            ("fix: release [publish only]", (True, True, False)),
            ("fix: release [PUBLISH ONLY]", (True, True, False)),
            ("fix: release\n\n[publish only]", (True, True, False)),
        ]
        for prefix in ("docs:", "doc:", "tests:", "test:", "ci:"):
            cases.append((prefix + " existing skip", (False, False, False)))
            cases.append((prefix + " existing skip [publish only]", (False, False, False)))
        for message, expected in cases:
            with self.subTest(message=message):
                self.assertEqual(release_job_decisions(ROOT / ".github/workflows/ci.yml", message), expected)


class BrowserSentryBuildArgsContract(unittest.TestCase):
    """BrowserSentryBuildArgsContract exercises bounded public inputs and unchanged release controls."""

    FIXTURE_DSN = "https://" + "a" * 32 + "@o123.ingest.sentry.io/456"
    FIXTURE_SHA = "b" * 40
    APPROVED_WORKFLOW_SHA256 = "4ca28bd752d238557e0d3b08052b6a1a2b8760dfbf792ee6a421c643e01819c5"
    BRIDGE_STEP = """      - name: Prepare public chat browser build arguments
        id: chat_sentry
        env:
          CHAT_SENTRY_ENABLED: ${{ vars.CHAT_SENTRY_ENABLED }}
          CHAT_SENTRY_DSN: ${{ vars.CHAT_SENTRY_DSN }}
          CHAT_SENTRY_ENVIRONMENT: ${{ vars.CHAT_SENTRY_ENVIRONMENT }}
          CHAT_SENTRY_SOURCE_SHA: ${{ github.sha }}
        run: python3 ci/browser_sentry_build_args.py

"""
    BUILD_ARGS_INPUT = "          build-args: ${{ steps.chat_sentry.outputs.build_args }}\n"

    def enabled_environment(self):
        """enabled_environment returns synthetic public values without any real project configuration."""
        return {
            "CHAT_SENTRY_ENABLED": "true",
            "CHAT_SENTRY_DSN": self.FIXTURE_DSN,
            "CHAT_SENTRY_ENVIRONMENT": "production",
            "CHAT_SENTRY_SOURCE_SHA": self.FIXTURE_SHA,
        }

    def disabled_arguments(self):
        """disabled_arguments returns the fixed four-argument disabled contract."""
        return (
            "VITE_CHAT_SENTRY_ENABLED=false\n"
            "VITE_CHAT_SENTRY_DSN=\n"
            "VITE_CHAT_SENTRY_ENVIRONMENT=\n"
            "VITE_CHAT_SENTRY_RELEASE="
        )

    def test_missing_or_nontrue_opt_in_keeps_all_public_values_blank(self):
        """test_missing_or_nontrue_opt_in_keeps_all_public_values_blank prevents accidental activation or value forwarding."""
        for opt_in in ("", "false", "TRUE", "1", "synthetic-private-value"):
            environ = self.enabled_environment()
            environ["CHAT_SENTRY_ENABLED"] = opt_in
            environ["CHAT_SENTRY_DSN"] = "synthetic-private-value"
            environ["CHAT_SENTRY_ENVIRONMENT"] = "synthetic-private-value"
            environ["CHAT_SENTRY_SOURCE_SHA"] = "synthetic-private-value"
            self.assertEqual(prepare_browser_build_args(environ), self.disabled_arguments())
        self.assertEqual(prepare_browser_build_args({}), self.disabled_arguments())

    def test_valid_public_input_derives_release_only_from_workflow_sha(self):
        """test_valid_public_input_derives_release_only_from_workflow_sha ignores arbitrary environment and release overrides."""
        environ = self.enabled_environment()
        environ.update({
            "CHAT_SENTRY_RELEASE": "synthetic-private-value",
            "VITE_CHAT_SENTRY_RELEASE": "synthetic-private-value",
            "SENTRY_AUTH_TOKEN": "synthetic-private-value",
            "UNRELATED_SECRET": "synthetic-private-value",
        })
        args = prepare_browser_build_args(environ)
        self.assertEqual(len(args.splitlines()), 4)
        self.assertIn("VITE_CHAT_SENTRY_RELEASE=go-ramjet@" + self.FIXTURE_SHA, args)
        self.assertNotIn("synthetic-private-value", args)
        self.assertNotIn("AUTH_TOKEN", args)
        for region in ("", ".us", ".de"):
            environ["CHAT_SENTRY_DSN"] = self.FIXTURE_DSN.replace(".ingest.", ".ingest" + region + ".")
            self.assertIn("VITE_CHAT_SENTRY_ENABLED=true", prepare_browser_build_args(environ))

    def test_public_value_bounds_accept_boundary_and_reject_overflow(self):
        """test_public_value_bounds_accept_boundary_and_reject_overflow verifies the exact environment and DSN size limits."""
        environ = self.enabled_environment()
        environ["CHAT_SENTRY_ENVIRONMENT"] = "x" * 32
        self.assertIn("VITE_CHAT_SENTRY_ENABLED=true", prepare_browser_build_args(environ))
        # Synthetic syntax-only fixture; no DNS lookup, SDK init or event occurs.
        prefix = "https://" + "a" * 32 + "@o"
        suffix = ".ingest.sentry.io/456"
        environ["CHAT_SENTRY_DSN"] = prefix + "1" * (256 - len(prefix + suffix)) + suffix
        self.assertEqual(len(environ["CHAT_SENTRY_DSN"]), 256)
        self.assertIn("VITE_CHAT_SENTRY_ENABLED=true", prepare_browser_build_args(environ))
        environ["CHAT_SENTRY_DSN"] += "1"
        with self.assertRaises(PublicConfigurationError):
            prepare_browser_build_args(environ)

    def test_real_helper_disabled_output_never_forwards_submitted_values(self):
        """test_real_helper_disabled_output_never_forwards_submitted_values verifies the actual runner behavior when opt-in is absent."""
        with tempfile.TemporaryDirectory(prefix="chat-public-args-") as directory:
            output_path = Path(directory) / "output"
            environ = {"CHAT_SENTRY_DSN": "synthetic-private-value", "SENTRY_AUTH_TOKEN": "synthetic-private-value"}
            result = self.run_preparation(environ, output_path)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")
            self.assertEqual(result.stderr, "")
            self.assertEqual("\n".join(output_path.read_text().splitlines()[1:-1]), self.disabled_arguments())
            self.assertNotIn("synthetic-private-value", output_path.read_text())

    def test_explicit_opt_in_rejects_missing_or_unsafe_public_values(self):
        """test_explicit_opt_in_rejects_missing_or_unsafe_public_values fails closed without reflecting malformed input."""
        cases = [
            ("CHAT_SENTRY_DSN", ""),
            ("CHAT_SENTRY_DSN", "x" * 257),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN.replace("https:", "http:")),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN.replace("@", ":synthetic-private-value@")),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN.replace("sentry.io", "sentry.io.evil.test")),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN.replace("/456", ":443/456")),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN + "?token=synthetic-private-value"),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN + "#synthetic-private-value"),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN.replace("/456", "/%34%35%36")),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN.replace("https://" + "a", "https://%61", 1)),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN + "\nSENTRY_AUTH_TOKEN=synthetic-private-value"),
            ("CHAT_SENTRY_DSN", self.FIXTURE_DSN + "\r"),
            ("CHAT_SENTRY_ENVIRONMENT", ""),
            ("CHAT_SENTRY_ENVIRONMENT", "x" * 33),
            ("CHAT_SENTRY_ENVIRONMENT", "production\nsynthetic-private-value"),
            ("CHAT_SENTRY_ENVIRONMENT", "private@example.com"),
            ("CHAT_SENTRY_SOURCE_SHA", ""),
            ("CHAT_SENTRY_SOURCE_SHA", "b" * 39),
            ("CHAT_SENTRY_SOURCE_SHA", "B" * 40),
            ("CHAT_SENTRY_SOURCE_SHA", "b" * 40 + "\nsynthetic-private-value"),
        ]
        for key, value in cases:
            with self.subTest(key=key, scenario=cases.index((key, value))):
                environ = self.enabled_environment()
                environ[key] = value
                with self.assertRaises(PublicConfigurationError) as caught:
                    prepare_browser_build_args(environ)
                self.assertEqual(str(caught.exception), INVALID_MESSAGE)

    def run_preparation(self, environ, output_path):
        """run_preparation executes the real helper with an isolated runner environment and captures diagnostics."""
        import sys
        return subprocess.run(
            [sys.executable, str(ROOT / "ci/browser_sentry_build_args.py")],
            env={**environ, "GITHUB_OUTPUT": str(output_path)},
            text=True, capture_output=True, timeout=5,
        )

    def test_real_helper_writes_bounded_actions_output_without_logging_values(self):
        """test_real_helper_writes_bounded_actions_output_without_logging_values verifies actual runner output and silent success."""
        with tempfile.TemporaryDirectory(prefix="chat-public-args-") as directory:
            output_path = Path(directory) / "output"
            result = self.run_preparation(self.enabled_environment(), output_path)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(result.stdout, "")
            self.assertEqual(result.stderr, "")
            lines = output_path.read_text().splitlines()
            self.assertEqual(lines[0], "build_args<<" + OUTPUT_DELIMITER)
            self.assertEqual(lines[-1], OUTPUT_DELIMITER)
            self.assertEqual(len(lines), 6)
            self.assertEqual("\n".join(lines[1:-1]), prepare_browser_build_args(self.enabled_environment()))

    def test_real_helper_invalid_opt_in_does_not_write_or_echo_submitted_data(self):
        """test_real_helper_invalid_opt_in_does_not_write_or_echo_submitted_data proves a publishing job cannot consume invalid outputs."""
        with tempfile.TemporaryDirectory(prefix="chat-public-args-") as directory:
            output_path = Path(directory) / "output"
            environ = self.enabled_environment()
            environ["CHAT_SENTRY_DSN"] = "synthetic-private-value\nSENTRY_AUTH_TOKEN=synthetic-private-value"
            result = self.run_preparation(environ, output_path)
            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, "")
            self.assertEqual(result.stderr, INVALID_MESSAGE + "\n")
            self.assertFalse(output_path.exists())

    def test_real_helper_output_failure_is_static(self):
        """test_real_helper_output_failure_is_static contains runner file errors without exposing paths or configuration."""
        with tempfile.TemporaryDirectory(prefix="chat-public-args-") as directory:
            result = self.run_preparation(self.enabled_environment(), Path(directory) / "missing" / "output")
            self.assertEqual(result.returncode, 1)
            self.assertEqual(result.stdout, "")
            self.assertEqual(result.stderr, INVALID_MESSAGE + "\n")

    def test_bridge_is_identical_in_both_builds_and_preserves_every_release_control(self):
        """test_bridge_is_identical_in_both_builds_and_preserves_every_release_control requires the approved workflow after removing only the public bridge."""
        import hashlib
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        self.assertEqual(source.count(self.BRIDGE_STEP), 2)
        self.assertEqual(source.count(self.BUILD_ARGS_INPUT), 2)
        for job in ("build_latest", "build_latest_cache"):
            body = source.split("  " + job + ":\n", 1)[1]
            body = re.split(r"^  [a-zA-Z_][a-zA-Z_0-9-]*:\n", body, maxsplit=1, flags=re.MULTILINE)[0]
            self.assertIn(self.BRIDGE_STEP, body)
            self.assertIn(self.BUILD_ARGS_INPUT, body)
        preserved = source.replace(self.BRIDGE_STEP, "").replace(self.BUILD_ARGS_INPUT, "")
        self.assertEqual(hashlib.sha256(preserved.encode()).hexdigest(), self.APPROVED_WORKFLOW_SHA256)

    def test_feature_branch_push_remains_outside_release_trigger(self):
        """test_feature_branch_push_remains_outside_release_trigger guards the current Draft branch against publishing or deployment."""
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        branch_section = source.split("    branches:\n", 1)[1].split("    paths-ignore:\n", 1)[0]
        branches = re.findall(r"^      - '([^']+)'$", branch_section, re.MULTILINE)
        self.assertEqual(branches, ["master", "main", "test/ci"])
        self.assertNotIn("feat/browser-sentry-20261012-08", branches)


if __name__ == "__main__":
    unittest.main(verbosity=2)
