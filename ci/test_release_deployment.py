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


def read_workflow(path):
    """read_workflow parses a workflow Path and returns its YAML mapping."""
    import yaml
    workflow = yaml.safe_load(path.read_text())
    # PyYAML uses YAML 1.1, where the Actions key "on" is a boolean.
    if "on" not in workflow and True in workflow:
        workflow["on"] = workflow.pop(True)
    return workflow


def runs_python_script(command, script, bytecode_free=False):
    """runs_python_script checks the actual argv for a Python script invocation."""
    args = shlex.split(command, comments=True)
    return (len(args) in (2, 3) and Path(args[0]).name == "python3" and args[-1] == script
            and all(arg == "-B" for arg in args[1:-1]) and (not bytecode_free or "-B" in args))


def extract_release_script(workflow):
    """extract_release_script returns the deployment job's unique SSH script."""
    steps = read_workflow(workflow)["jobs"]["deploy"]["steps"]
    scripts = [step["with"]["script"] for step in steps if "script" in step.get("with", {})]
    if len(scripts) != 1:
        raise ValueError("expected exactly one remote deployment script")
    return scripts[0]


def release_job_decisions(workflow, message):
    """release_job_decisions executes the prefix check and evaluates release selectors."""
    jobs = read_workflow(workflow)["jobs"]
    scripts = [step["run"] for step in jobs["check_skip"]["steps"] if step.get("id") == "check"]
    if len(scripts) != 1:
        raise AssertionError("Expected exactly one commit-prefix check script")
    with tempfile.TemporaryDirectory(prefix="release-selector-contract-") as directory:
        output = Path(directory) / "outputs"
        result = subprocess.run(
            ["bash"], input=scripts[0], text=True, capture_output=True, timeout=5,
            env={"PATH": os.environ["PATH"], "COMMIT_MSG": message, "GITHUB_OUTPUT": str(output)},
        )
        if result.returncode:
            raise AssertionError(result.stderr)
        outputs = dict(line.split("=", 1) for line in output.read_text().splitlines())
    skip = outputs["should_skip"]

    def selected(name):
        """selected evaluates the existing restricted release expression grammar."""
        condition = jobs[name]["if"].strip()
        if condition.startswith("${{") and condition.endswith("}}"):
            condition = condition[3:-2].strip()
        decisions = []
        for predicate in re.split(r"\s*&&\s*", condition):
            predicate = " ".join(predicate.split())
            if predicate == "needs.check_skip.outputs.should_skip != 'true'":
                decisions.append(skip.casefold() != "true")
                continue
            marker = re.fullmatch(r"!contains\(github\.event\.head_commit\.message, '([^']*)'\)", predicate)
            if marker is None:
                raise AssertionError("Unsupported release selector: " + predicate)
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
        """test_pr_validation_runs_contract_before_go_unit_tests checks execution order."""
        steps = read_workflow(ROOT / ".github/workflows/pr-validation.yml")["jobs"]["backend"]["steps"]
        contracts = [index for index, step in enumerate(steps)
                     if runs_python_script(step.get("run", ""), "ci/test_release_deployment.py", bytecode_free=True)]
        go_tests = [index for index, step in enumerate(steps)
                    if shlex.split(step.get("run", "")) == ["go", "test", "./...", "-count=1"]]
        self.assertEqual(len(contracts), 1)
        self.assertEqual(len(go_tests), 1)
        self.assertLess(contracts[0], go_tests[0])

    def test_helper_only_changes_run_unfiltered_pr_contract_validation(self):
        """test_helper_only_changes_run_unfiltered_pr_contract_validation checks event coverage."""
        workflow = read_workflow(ROOT / ".github/workflows/pr-validation.yml")
        trigger = workflow["on"]["pull_request"]
        self.assertIn("master", trigger["branches"])
        self.assertNotIn("paths", trigger)
        self.assertNotIn("paths-ignore", trigger)
        self.assertTrue(any(runs_python_script(step.get("run", ""), "ci/test_release_deployment.py", bytecode_free=True)
                            for step in workflow["jobs"]["backend"]["steps"]))

    def test_release_selectors_deploy_marked_releases_and_preserve_prefix_skips(self):
        """test_release_selectors_deploy_marked_releases_and_preserve_prefix_skips runs actual release selectors."""
        cases = [
            ("fix: ordinary release", (True, True, True)),
            ("fix: docs wording without a skipped prefix", (True, True, True)),
            ("fix: quoted '$()' commit text", (True, True, True)),
            ("fix: release [publish only]", (True, True, True)),
            ("fix: release [PUBLISH ONLY]", (True, True, True)),
            ("fix: release\n\n[publish only]", (True, True, True)),
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


class ReleaseWorkflowWiringContract(unittest.TestCase):
    def assert_required_step(self, step):
        """assert_required_step checks a required action executes and propagates failures."""
        self.assertIs(step.get("continue-on-error", False), False)
        condition = step.get("if", True)
        self.assertIn("".join(str(condition).split()).lower(),
                      {"true", "success()", "${{true}}", "${{success()}}"})

    """ReleaseWorkflowWiringContract checks delivery behavior fields rather than YAML bytes."""

    def assert_build_jobs(self, workflow):
        """assert_build_jobs checks both public bridges, publication roles and target platforms."""
        for name, publishes in (("build_latest", True), ("build_latest_cache", False)):
            steps = workflow["jobs"][name]["steps"]
            helpers = [(index, step) for index, step in enumerate(steps) if step.get("id") == "chat_sentry"]
            builds = [(index, step) for index, step in enumerate(steps)
                      if step.get("uses", "").startswith("docker/build-push-action@")]
            self.assertEqual(len(helpers), 1)
            self.assertEqual(len(builds), 1)
            helper_index, helper = helpers[0]
            build_index, build = builds[0]
            self.assertLess(helper_index, build_index)
            self.assert_required_step(helper)
            self.assert_required_step(build)
            self.assertEqual(helper["env"], {
                "CHAT_SENTRY_ENABLED": "${{ vars.CHAT_SENTRY_ENABLED }}",
                "CHAT_SENTRY_DSN": "${{ vars.CHAT_SENTRY_DSN }}",
                "CHAT_SENTRY_ENVIRONMENT": "${{ vars.CHAT_SENTRY_ENVIRONMENT }}",
                "CHAT_SENTRY_SOURCE_SHA": "${{ github.sha }}",
            })
            self.assertTrue(runs_python_script(helper["run"], "ci/browser_sentry_build_args.py"))
            inputs = build["with"]
            self.assertEqual(inputs["context"], ".")
            self.assertEqual("".join(inputs["build-args"].split()), "${{steps.chat_sentry.outputs.build_args}}")
            self.assertEqual(str(inputs["push"]).lower(), str(publishes).lower())
            platforms = set(str(inputs.get("platforms", "linux/amd64")).replace(",", "\n").split())
            self.assertEqual(platforms, {"linux/amd64"})
            tags = set(inputs.get("tags", "").splitlines())
            if publishes:
                self.assertEqual(tags, {"ppcelery/go-ramjet:latest",
                                        "ppcelery/go-ramjet:${{ steps.vars.outputs.short_sha }}"})
            else:
                self.assertFalse(tags)
                self.assertIn("type=gha", inputs["cache-from"])
                self.assertIn("type=gha", inputs["cache-to"])

    def assert_release_controls(self, workflow):
        """assert_release_controls checks triggers, successful-build dependencies and serialization."""
        self.assertEqual(set(workflow["on"]), {"push"})
        trigger = workflow["on"]["push"]
        self.assertEqual(set(trigger["branches"]), {"master", "main", "test/ci"})
        self.assertEqual(set(trigger["paths-ignore"]), {"docs/**", "doc/**", "tests/**", "test/**", "ci/**"})
        jobs = workflow["jobs"]
        for name in ("check_skip", "build_latest", "build_latest_cache", "deploy"):
            self.assertIs(jobs[name].get("continue-on-error", False), False, name)
        checks = [step for step in jobs["check_skip"]["steps"] if step.get("id") == "check"]
        self.assertEqual(len(checks), 1)
        self.assert_required_step(checks[0])
        self.assertEqual("".join(checks[0]["env"]["COMMIT_MSG"].split()),
                         "${{github.event.head_commit.message}}")
        self.assertEqual("".join(jobs["check_skip"]["outputs"]["should_skip"].split()),
                         "${{steps.check.outputs.should_skip}}")
        variables = [step for step in jobs["build_latest"]["steps"] if step.get("id") == "vars"]
        self.assertEqual(len(variables), 1)
        self.assert_required_step(variables[0])
        for name, required in (("build_latest", {"check_skip"}),
                               ("build_latest_cache", {"check_skip", "build_latest"}),
                               ("deploy", {"check_skip", "build_latest"})):
            needs = jobs[name]["needs"]
            needs = {needs} if isinstance(needs, str) else set(needs)
            self.assertTrue(required.issubset(needs), name)
        concurrency = jobs["deploy"]["concurrency"]
        self.assertIsInstance(concurrency["group"], str)
        self.assertTrue(concurrency["group"].strip())
        self.assertNotIn("${{", concurrency["group"])
        self.assertIs(concurrency["cancel-in-progress"], False)

    def assert_ssh_binding(self, workflow):
        """assert_ssh_binding checks existing remote references and a unique deployment script."""
        ssh = [step for step in workflow["jobs"]["deploy"]["steps"]
               if step.get("uses", "").startswith("appleboy/ssh-action@")]
        self.assertEqual(len(ssh), 1)
        self.assert_required_step(ssh[0])
        inputs = ssh[0]["with"]
        self.assertEqual({key: inputs[key] for key in ("host", "username", "password", "port")}, {
            "host": "${{ secrets.TARGET_HOST }}", "username": "${{ secrets.TARGET_HOST_USERNAME }}",
            "password": "${{ secrets.TARGET_HOST_PASSWORD }}", "port": "${{ secrets.TARGET_HOST_SSH_PORT }}",
        })
        self.assertIsInstance(inputs["script"], str)
        self.assertTrue(inputs["script"].strip())

    def assert_delivery_wiring(self, workflow):
        """assert_delivery_wiring runs only the targeted delivery-role assertions."""
        self.assert_build_jobs(workflow)
        self.assert_release_controls(workflow)
        self.assert_ssh_binding(workflow)

    def test_public_browser_inputs_are_wired_into_both_builds(self):
        """test_public_browser_inputs_are_wired_into_both_builds checks publication and cache roles."""
        self.assert_build_jobs(read_workflow(ROOT / ".github/workflows/ci.yml"))

    def test_release_triggers_dependencies_and_serialization(self):
        """test_release_triggers_dependencies_and_serialization checks the approved release boundary."""
        self.assert_release_controls(read_workflow(ROOT / ".github/workflows/ci.yml"))

    def test_ssh_deployment_keeps_existing_binding(self):
        """test_ssh_deployment_keeps_existing_binding checks unchanged target credential references."""
        self.assert_ssh_binding(read_workflow(ROOT / ".github/workflows/ci.yml"))

    def test_commit_image_tag_uses_running_workflow_sha(self):
        """test_commit_image_tag_uses_running_workflow_sha executes the actual short-SHA output step."""
        steps = read_workflow(ROOT / ".github/workflows/ci.yml")["jobs"]["build_latest"]["steps"]
        scripts = [step["run"] for step in steps if step.get("id") == "vars"]
        self.assertEqual(len(scripts), 1)
        with tempfile.TemporaryDirectory(prefix="ramjet-image-tag-") as directory:
            output = Path(directory) / "output"
            result = subprocess.run(["bash"], input=scripts[0], text=True, capture_output=True, timeout=5,
                                    env={"PATH": os.environ["PATH"], "GITHUB_SHA": "b" * 40,
                                         "GITHUB_OUTPUT": str(output)})
            self.assertEqual(result.returncode, 0)
            self.assertEqual(output.read_text().splitlines(), ["short_sha=" + "b" * 7])

    def test_comments_quoting_and_display_names_preserve_delivery_contract(self):
        """test_comments_quoting_and_display_names_preserve_delivery_contract keeps cosmetic edits valid."""
        import yaml
        source = (ROOT / ".github/workflows/ci.yml").read_text()
        variants = (
            "# Cosmetic workflow comment.\n" + source,
            source.replace("      - 'master'", '      - "master"', 1),
            source.replace("      cancel-in-progress: false", "      cancel-in-progress: false # Keep active rollout", 1),
            source.replace("name: Prepare public chat browser build arguments", "name: Prepare browser reporting"),
        )
        for variant in variants:
            workflow = yaml.safe_load(variant)
            if "on" not in workflow and True in workflow:
                workflow["on"] = workflow.pop(True)
            self.assert_delivery_wiring(workflow)
        workflow["jobs"]["build_latest"]["steps"].insert(0, {"name": "Explain build", "run": "true"})
        workflow["jobs"]["deploy"]["steps"].insert(0, {"name": "Explain rollout", "run": "true"})
        self.assert_delivery_wiring(workflow)

    def test_real_publication_target_and_security_changes_are_rejected(self):
        """test_real_publication_target_and_security_changes_are_rejected preserves substantive gates."""
        import copy
        workflow = read_workflow(ROOT / ".github/workflows/ci.yml")

        def build(model, job="build_latest"):
            """build returns image action inputs for a fixture mutation."""
            return next(step["with"] for step in model["jobs"][job]["steps"]
                        if step.get("uses", "").startswith("docker/build-push-action@"))

        def bridge(model):
            """bridge returns the public helper step for a fixture mutation."""
            return next(step for step in model["jobs"]["build_latest"]["steps"] if step.get("id") == "chat_sentry")

        def delay_bridge(model):
            """delay_bridge moves the identified helper behind its consumer."""
            steps = model["jobs"]["build_latest"]["steps"]
            steps.append(steps.pop(next(index for index, step in enumerate(steps)
                                        if step.get("id") == "chat_sentry")))

        def ssh_step(model):
            """ssh_step locates the deployment action independently of display ordering."""
            return next(step for step in model["jobs"]["deploy"]["steps"]
                        if step.get("uses", "").startswith("appleboy/ssh-action@"))

        mutations = (
            ("skip-input-disconnected", lambda model: next(step for step in model["jobs"]["check_skip"]["steps"] if step.get("id") == "check")["env"].update(COMMIT_MSG="constant")),
            ("skip-output-disconnected", lambda model: model["jobs"]["check_skip"]["outputs"].update(should_skip="false")),
            ("helper-skipped", lambda model: bridge(model).update({"if": False})),
            ("helper-failure-ignored", lambda model: bridge(model).update({"continue-on-error": True})),
            ("build-failure-ignored", lambda model: next(step for step in model["jobs"]["build_latest"]["steps"] if step.get("uses", "").startswith("docker/build-push-action@")).update({"continue-on-error": True})),
            ("deploy-failure-ignored", lambda model: model["jobs"]["deploy"].update({"continue-on-error": True})),
            ("publish-disabled", lambda model: build(model).update(push=False)),
            ("cache-published", lambda model: build(model, "build_latest_cache").update(push=True)),
            ("wrong-tags", lambda model: build(model).update(tags="another/image:latest")),
            ("wrong-platform", lambda model: build(model).update(platforms="linux/arm64")),
            ("missing-build-dependency", lambda model: model["jobs"]["deploy"].update(needs=["check_skip"])),
            ("feature-branch-release", lambda model: model["on"]["push"]["branches"].append("feature/*")),
            ("new-publish-event", lambda model: model["on"].update(pull_request={})),
            ("canceled-rollout", lambda model: model["jobs"]["deploy"]["concurrency"].update({"cancel-in-progress": True})),
            ("branch-specific-rollout", lambda model: model["jobs"]["deploy"]["concurrency"].update(group="deploy-${{ github.ref }}")),
            ("secret-browser-input", lambda model: bridge(model)["env"].update(CHAT_SENTRY_DSN="${{ secrets.PRIVATE_DSN }}")),
            ("wrong-output", lambda model: build(model).update({"build-args": "${{ steps.unrelated.outputs.build_args }}"})),
            ("late-helper", delay_bridge),
            ("changed-ssh-target", lambda model: ssh_step(model)["with"].update(host="unreviewed-host")),
            ("ssh-skipped", lambda model: ssh_step(model).update({"if": False})),
            ("ssh-failure-ignored", lambda model: ssh_step(model).update({"continue-on-error": True})),
        )
        for name, mutate in mutations:
            with self.subTest(name=name):
                candidate = copy.deepcopy(workflow)
                mutate(candidate)
                with self.assertRaises(AssertionError):
                    self.assert_delivery_wiring(candidate)

    def test_broader_recreation_and_masked_failures_fail_executable_contracts(self):
        """test_broader_recreation_and_masked_failures_fail_executable_contracts runs changed shells."""
        import copy
        import yaml
        from unittest import mock
        original = read_workflow(ROOT / ".github/workflows/ci.yml")
        fixtures = (
            ("--no-deps --force-recreate go-ramjet", "--force-recreate",
             "test_recreates_only_ramjet_without_dependencies_or_orphan_removal"),
            ("set -Eeuo pipefail", "set +e", "test_up_failure_cannot_be_masked_by_later_commands"),
        )
        for old, new, method in fixtures:
            with self.subTest(method=method), tempfile.TemporaryDirectory(prefix="ramjet-unsafe-shell-") as directory:
                workflow = copy.deepcopy(original)
                ssh = next(step["with"] for step in workflow["jobs"]["deploy"]["steps"]
                           if step.get("uses", "").startswith("appleboy/ssh-action@"))
                self.assertEqual(ssh["script"].count(old), 1)
                ssh["script"] = ssh["script"].replace(old, new, 1)
                path = Path(directory) / ".github/workflows/ci.yml"
                path.parent.mkdir(parents=True)
                path.write_text(yaml.safe_dump(workflow))
                contract = ReleaseDeploymentContract(method)
                contract.setUp()
                try:
                    with mock.patch.dict(globals(), {"ROOT": Path(directory)}):
                        with self.assertRaises(AssertionError):
                            getattr(contract, method)()
                finally:
                    contract.doCleanups()


if __name__ == "__main__":
    unittest.main(verbosity=2)
