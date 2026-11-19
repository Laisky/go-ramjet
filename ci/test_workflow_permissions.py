#!/usr/bin/env python3
"""Contracts that keep every workflow's GITHUB_TOKEN scopes explicit and minimal."""

import copy
from pathlib import Path
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]
WORKFLOWS = ROOT / ".github" / "workflows"

# EXPECTED_WRITE_GRANTS lists every reviewed (job, scope) pair that may hold write access.
EXPECTED_WRITE_GRANTS = {
    "bolt-verify.yml": {("report", "statuses")},
    "ci.yml": set(),
    "pr-validation.yml": set(),
    "sync-go127-tidy.yml": {("sync", "contents")},
}

# RELEASE_JOB_PERMISSIONS is the exact effective token scope of each release job.
RELEASE_JOB_PERMISSIONS = {
    "check_skip": {},
    "build_latest": {"contents": "read"},
    "build_latest_cache": {"contents": "read"},
    "deploy": {},
}


def read_workflow(path):
    """read_workflow parses a workflow Path and returns its YAML mapping."""
    workflow = yaml.safe_load(path.read_text())
    # PyYAML uses YAML 1.1, where the Actions key "on" is a boolean.
    if "on" not in workflow and True in workflow:
        workflow["on"] = workflow.pop(True)
    return workflow


def effective_permissions(workflow, job_name):
    """effective_permissions returns a job's token scopes, which replace the workflow default when declared."""
    job = workflow["jobs"][job_name]
    return job["permissions"] if "permissions" in job else workflow.get("permissions")


def write_grants(workflow):
    """write_grants returns every (job, scope) pair with write access, rejecting implicit or broad grants."""
    if "permissions" not in workflow:
        raise AssertionError("workflow must declare top-level permissions")
    grants = set()
    for job_name in workflow["jobs"]:
        permissions = effective_permissions(workflow, job_name)
        if not isinstance(permissions, dict):
            raise AssertionError(f"{job_name} must use an explicit permission mapping, not {permissions!r}")
        for scope, access in permissions.items():
            if access not in ("read", "write", "none"):
                raise AssertionError(f"{job_name} has unsupported {scope} access {access!r}")
            if access == "write":
                grants.add((job_name, scope))
    return grants


class WorkflowTokenPermissionContract(unittest.TestCase):
    """WorkflowTokenPermissionContract checks parsed permission blocks rather than YAML bytes."""

    def assert_release_permissions(self, workflow):
        """assert_release_permissions checks deny-by-default release scopes and exact per-job grants."""
        self.assertEqual(workflow.get("permissions"), {})
        self.assertEqual(write_grants(workflow), set())
        for job_name, expected in RELEASE_JOB_PERMISSIONS.items():
            self.assertEqual(effective_permissions(workflow, job_name), expected, job_name)

    def test_every_workflow_declares_reviewed_token_scopes(self):
        """test_every_workflow_declares_reviewed_token_scopes rejects unreviewed workflows and write grants."""
        names = sorted(path.name for path in WORKFLOWS.glob("*.y*ml"))
        self.assertEqual(names, sorted(EXPECTED_WRITE_GRANTS))
        for name in names:
            with self.subTest(workflow=name):
                self.assertEqual(write_grants(read_workflow(WORKFLOWS / name)), EXPECTED_WRITE_GRANTS[name])

    def test_release_workflow_grants_only_checkout_reads(self):
        """test_release_workflow_grants_only_checkout_reads checks the release jobs' exact token scopes."""
        self.assert_release_permissions(read_workflow(WORKFLOWS / "ci.yml"))

    def test_broad_or_unreviewed_grants_are_rejected(self):
        """test_broad_or_unreviewed_grants_are_rejected proves the contracts fail on real permission regressions."""
        release = read_workflow(WORKFLOWS / "ci.yml")
        bolt = read_workflow(WORKFLOWS / "bolt-verify.yml")
        release_mutations = (
            ("implicit-default", lambda model: model.pop("permissions")),
            ("write-all-default", lambda model: model.update(permissions="write-all")),
            ("read-all-default", lambda model: model.update(permissions="read-all")),
            ("default-contents-read", lambda model: model.update(permissions={"contents": "read"})),
            ("package-publish", lambda model: model["jobs"]["build_latest"]["permissions"].update(packages="write")),
            ("oidc-token", lambda model: model["jobs"]["build_latest"]["permissions"].update({"id-token": "write"})),
            ("cache-job-write", lambda model: model["jobs"]["build_latest_cache"]["permissions"].update(actions="write")),
            ("deploy-contents", lambda model: model["jobs"]["deploy"].update(permissions={"contents": "read"})),
            ("skip-check-contents", lambda model: model["jobs"]["check_skip"].update(permissions={"contents": "read"})),
        )
        for name, mutate in release_mutations:
            with self.subTest(name=name):
                candidate = copy.deepcopy(release)
                mutate(candidate)
                with self.assertRaises(AssertionError):
                    self.assert_release_permissions(candidate)

        bolt_mutations = (
            ("status-write-default", lambda model: model["permissions"].update(statuses="write")),
            ("verification-status-write", lambda model: model["jobs"]["repository-verification"].update(
                permissions={"contents": "read", "statuses": "write"})),
            ("report-contents-write", lambda model: model["jobs"]["report"]["permissions"].update(contents="write")),
        )
        for name, mutate in bolt_mutations:
            with self.subTest(name=name):
                candidate = copy.deepcopy(bolt)
                mutate(candidate)
                self.assertNotEqual(write_grants(candidate), EXPECTED_WRITE_GRANTS["bolt-verify.yml"])


if __name__ == "__main__":
    unittest.main(verbosity=2)
