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


def main():
    """main binds the checked-out tree to the expected PR head and reports only safe facts."""
    try:
        expected = os.environ['CV_PRINT_PROBE_EXPECTED_HEAD']
        if not re.fullmatch(r'[0-9a-f]{40}', expected):
            raise probe.ProbeError('expected_pr_head_invalid')
        if probe.git(ROOT, 'rev-parse', expected + '^{tree}') != probe.git(ROOT, 'rev-parse', 'HEAD^{tree}'):
            raise probe.ProbeError('checkout_tree_differs_from_expected_pr_head')
        row = run_smoke(ROOT)
        print('CV_PRINT_SMOKE ' + json.dumps({key: row[key] for key in
              ('status', 'originalPassCount', 'cleanupComplete')}, sort_keys=True))
        return 0 if row['status'] == 'passed' else 1
    except (KeyError, probe.ProbeError):
        print('CV_PRINT_SMOKE setup_failed')
        return 1


if __name__ == '__main__':
    sys.exit(main())
