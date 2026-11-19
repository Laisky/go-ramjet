"""Run one current-source browser cell with the existing fail-closed coverage checks."""

import importlib.util
import json
import os
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('probe_cv_print', ROOT / 'ci/probe_cv_print.py')
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


def run_smoke(root):
    """run_smoke executes and evaluates the existing current auto browser cell."""
    cell = probe.CELLS[2]
    environment = os.environ.copy()
    environment.update(GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local')
    environment[probe.BROWSER_ENV] = 'auto'
    result = probe.run_browser(probe.command_for(False), cwd=root, env=environment,
                               timeout=probe.WATCHDOG_SECONDS)
    return probe.evaluate_cell(cell, result)


def require_checkout_contains_head(root, expected, base_ref):
    """require_checkout_contains_head accepts the expected PR head or GitHub's clean merge of it.

    A pull_request run checks out refs/pull/N/merge, whose tree differs from the head
    whenever the base branch has moved. That checkout is accepted only when it has exactly
    two parents, the first is on the fetched base branch base_ref, the second is the
    expected head, and its tree equals a clean merge of those parents. The tested source
    then contains every change in the head and nothing outside the base branch. Anything
    else raises a ProbeError.
    """
    head_tree = probe.git(root, 'rev-parse', 'HEAD^{tree}')
    if probe.git(root, 'rev-parse', expected + '^{tree}') == head_tree:
        return
    parents = probe.git(root, 'rev-parse', 'HEAD^@').split()
    if len(parents) != 2 or parents[1] != expected:
        raise probe.ProbeError('checkout_tree_differs_from_expected_pr_head')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._/-]*', base_ref) or '..' in base_ref:
        raise probe.ProbeError('expected_pr_base_invalid')
    # The payload's base SHA can predate the merge ref, so compare with the fetched branch tip.
    # merge-base and merge-tree exit non-zero on a failed check, which git() reports as a ProbeError.
    base_tip = probe.git(root, 'rev-parse', '--verify', 'refs/remotes/origin/' + base_ref + '^{commit}')
    probe.git(root, 'merge-base', '--is-ancestor', parents[0], base_tip)
    if probe.git(root, 'merge-tree', '--write-tree', *parents).splitlines()[:1] != [head_tree]:
        raise probe.ProbeError('checkout_tree_differs_from_expected_pr_head')


def main():
    """main binds the checkout to the expected PR head and reports only safe facts."""
    try:
        expected = os.environ['CV_PRINT_PROBE_EXPECTED_HEAD']
        if not re.fullmatch(r'[0-9a-f]{40}', expected):
            raise probe.ProbeError('expected_pr_head_invalid')
        require_checkout_contains_head(ROOT, expected, os.environ.get('CV_PRINT_PROBE_BASE_REF', ''))
        row = run_smoke(ROOT)
        print('CV_PRINT_SMOKE ' + json.dumps({key: row[key] for key in
              ('status', 'originalPassCount', 'cleanupComplete', 'missingOriginal',
               'failedActions', 'skippedActions', 'observations')}, sort_keys=True))
        return 0 if row['status'] == 'passed' else 1
    except (KeyError, probe.ProbeError):
        print('CV_PRINT_SMOKE setup_failed')
        return 1


if __name__ == '__main__':
    sys.exit(main())
