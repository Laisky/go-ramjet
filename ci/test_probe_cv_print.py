"""Pure driver contracts: no Go, browser, GitHub API, dependency install or network."""

import importlib.util
import json
import pathlib
import signal
import subprocess
import sys
import unittest
from unittest import mock

SPEC = importlib.util.spec_from_file_location('probe_cv_print', pathlib.Path(__file__).with_name('probe_cv_print.py'))
probe = importlib.util.module_from_spec(SPEC)
previous_bytecode = sys.dont_write_bytecode
try:
    sys.dont_write_bytecode = True
    SPEC.loader.exec_module(probe)
finally:
    sys.dont_write_bytecode = previous_bytecode


def synthetic_output(instrumented=False, mode='auto', omit=None, fail=None, skip=None):
    """synthetic_output returns mock Go JSON bytes with optional instrumentation, omissions, failures or skips."""
    rows = []
    for name in sorted(probe.ORIGINAL_TESTS):
        if name == omit:
            continue
        package = probe.CV_PACKAGE if name in probe.CV_TESTS else probe.TASK_PACKAGE
        if instrumented and name in probe.CV_TESTS:
            for text in (
                'CV_PRINT_PROBE browser_mode=' + mode,
                'CV_PRINT_PROBE runtime_parallelism=2 test_parallelism=2 cpu_count=2',
                'CV_PRINT_PROBE phase=print outcome=success phase_ms=10 total_ms=30 remaining_ms=29970 context=active browser_version=unavailable',
            ):
                rows.append({'Action': 'output', 'Package': package, 'Test': name, 'Output': 'fixture_test.go:20: ' + text + '\n'})
        rows.append({'Action': 'fail' if name == fail else 'skip' if name == skip else 'pass',
                     'Package': package, 'Test': name, 'Elapsed': 0.01})
    for package in probe.PACKAGES:
        rows.append({'Action': 'pass', 'Package': package, 'Elapsed': 0.1})
    return ('\n'.join(json.dumps(row) for row in rows) + '\n').encode()


def result(stdout, **changes):
    """result returns a synthetic process result with supplied output bytes and explicit field overrides."""
    value = {'exit': 0, 'stdout': stdout, 'stderr': b'', 'watchdogTimeout': False, 'wallDurationMS': 100}
    value.update(changes)
    return value


class DriverContracts(unittest.TestCase):
    def checkouts(self):
        """checkouts returns fixed owned paths and immutable source bindings for mocked matrix contracts."""
        return {name: {'path': pathlib.Path('/owned') / name,
                       'head': probe.ACCEPTED if name == 'accepted' else probe.EXACT,
                       'tree': '1' * 40} for name in ('accepted', 'exact', 'current')}

    def test_six_distinct_cells_keep_original_command_environment_and_coverage(self):
        """test_six_distinct_cells_keep_original_command_environment_and_coverage verifies six distinct cells keep original command environment and coverage using controlled fixtures."""
        calls = []
        def execute(args, cwd, env, timeout):
            """execute records supplied runner arguments and returns controlled output without starting a subprocess."""
            calls.append((args, cwd, env.copy(), timeout))
            instrumented = cwd.name == 'current'
            return result(synthetic_output(instrumented, env.get(probe.BROWSER_ENV, 'auto')))
        environment = {probe.BROWSER_ENV: 'untrusted-parent-value', 'GOMAXPROCS': '2',
                       'CGO_LDFLAGS': 'inherited-build-flags', 'ImageOS': 'ubuntu24',
                       'ImageVersion': '20261012.334.1'}
        report = probe.run_matrix(self.checkouts(), environment, execute=execute)
        self.assertTrue(report['allCellsPassed'])
        self.assertEqual(len(report['cells']), 6)
        self.assertEqual(len({row['cell'] for row in report['cells']}), 6)
        self.assertEqual(len(probe.ORIGINAL_TESTS), 35)
        self.assertEqual([row['originalPassCount'] for row in report['cells']], [35] * 6)
        self.assertEqual(sum('-p' in call[0] for call in calls), 2)
        for index, (args, cwd, env, timeout) in enumerate(calls):
            self.assertEqual(args[:5], ['dbus-run-session', '--', 'go', 'test', '-json'])
            self.assertIn(probe.SELECTOR, args)
            self.assertIn('./internal/tasks/cv', args)
            self.assertIn('./internal/tasks/gptchat/tasks', args)
            self.assertIn('-count=1', args)
            self.assertNotIn('-parallel', args)
            self.assertFalse(any(value.startswith('-timeout') for value in args))
            self.assertEqual(timeout, 240)
            self.assertEqual(env['GOMAXPROCS'], '2')
            self.assertEqual(env['CGO_LDFLAGS'], 'inherited-build-flags')
            self.assertEqual(env['GOPROXY'], 'off')
            self.assertEqual(env['GOSUMDB'], 'off')
            self.assertEqual(env['GOTOOLCHAIN'], 'local')
            self.assertEqual(env.get(probe.BROWSER_ENV), None if index < 2 else probe.CELLS[index][2])
        self.assertEqual(report['runnerMetadata'], {'ImageOS': 'ubuntu24', 'ImageVersion': '20261012.334.1'})
        self.assertNotIn('inherited-build-flags', json.dumps(report))
        self.assertEqual(environment[probe.BROWSER_ENV], 'untrusted-parent-value')

    def test_any_cell_failure_remains_failure_and_does_not_trigger_reruns(self):
        """test_any_cell_failure_remains_failure_and_does_not_trigger_reruns verifies any cell failure remains failure and does not trigger reruns using controlled fixtures."""
        calls = []
        def execute(args, cwd, env, timeout):
            """execute records supplied runner arguments and returns controlled output without starting a subprocess."""
            calls.append(cwd)
            return result(synthetic_output(cwd.name == 'current', env.get(probe.BROWSER_ENV, 'auto')),
                          exit=1 if len(calls) == 2 else 0)
        report = probe.run_matrix(self.checkouts(), {}, execute=execute)
        self.assertEqual(len(calls), 6)
        self.assertTrue(report['complete'])
        self.assertFalse(report['allCellsPassed'])
        self.assertEqual(report['cells'][1]['status'], 'test_failure')

    def test_original_identity_coverage_and_skips_cannot_be_replaced_by_counts(self):
        """test_original_identity_coverage_and_skips_cannot_be_replaced_by_counts verifies original identity coverage and skips cannot be replaced by counts using controlled fixtures."""
        for changes, expected in (
            ({'omit': 'TestRendererBrowserPrivateRequests/websocket'}, 'original_coverage_incomplete'),
            ({'skip': 'TestRendererBrowserUDPReachableNegativeControl'}, 'test_skip_not_accepted'),
            ({'fail': 'TestBrowserPDFWaitsForFontsAndKeepsPagination'}, 'test_failure'),
        ):
            with self.subTest(changes=changes):
                row = probe.evaluate_cell(probe.CELLS[0], result(synthetic_output(**changes)))
                self.assertEqual(row['status'], expected)

    def test_watchdog_is_explicit_even_if_partial_output_contains_all_passes(self):
        """test_watchdog_is_explicit_even_if_partial_output_contains_all_passes verifies watchdog is explicit even if partial output contains all passes using controlled fixtures."""
        row = probe.evaluate_cell(probe.CELLS[0], result(synthetic_output(), exit=-9, watchdogTimeout=True))
        self.assertEqual(row['status'], 'watchdog_timeout')
        self.assertTrue(row['watchdogTimeout'])
        process = mock.Mock(pid=12345, returncode=-9)
        process.communicate.side_effect = [subprocess.TimeoutExpired('owned-go-test', 240),
                                          subprocess.TimeoutExpired('owned-go-test', 5), (b'partial', b'opaque')]
        with mock.patch.object(probe.subprocess, 'Popen', return_value=process) as popen, \
                mock.patch.object(probe.os, 'killpg') as killpg:
            actual = probe.run_browser(['owned-command'], pathlib.Path('/owned'), {}, 240)
        self.assertTrue(actual['watchdogTimeout'])
        self.assertEqual(actual['stdout'], b'partial')
        self.assertEqual(killpg.call_args_list, [mock.call(12345, signal.SIGTERM), mock.call(12345, signal.SIGKILL)])
        self.assertTrue(popen.call_args.kwargs['start_new_session'])


    def test_watchdog_drain_hang_is_bounded_and_blocks_remaining_cells(self):
        """test_watchdog_drain_hang_is_bounded_and_blocks_remaining_cells verifies watchdog drain hang is bounded and blocks remaining cells using controlled fixtures."""
        process = mock.Mock(pid=12345, returncode=None)
        process.communicate.side_effect = [
            subprocess.TimeoutExpired('owned-go-test', 240),
            subprocess.TimeoutExpired('owned-go-test', 5),
            subprocess.TimeoutExpired('owned-go-test', 5, output=b'partial-json', stderr=b'opaque'),
        ]
        process.wait.side_effect = subprocess.TimeoutExpired('owned-go-test', 5)
        with mock.patch.object(probe.subprocess, 'Popen', return_value=process), \
                mock.patch.object(probe.os, 'killpg'):
            actual = probe.run_browser(['owned-command'], pathlib.Path('/owned'), {}, 240)
        self.assertTrue(actual['cleanupUnknown'])
        self.assertFalse(actual['cleanupComplete'])
        self.assertFalse(actual['streamCaptureComplete'])
        self.assertEqual(process.communicate.call_args_list,
                         [mock.call(timeout=240), mock.call(timeout=5), mock.call(timeout=5)])
        process.wait.assert_called_once_with(timeout=5)
        process.stdout.close.assert_called_once()
        process.stderr.close.assert_called_once()
        execute = mock.Mock(return_value=actual)
        report = probe.run_matrix(self.checkouts(), {}, execute=execute)
        self.assertEqual(execute.call_count, 1)
        self.assertFalse(report['complete'])
        self.assertFalse(report['allCellsPassed'])
        self.assertEqual(report['cells'][0]['status'], 'watchdog_cleanup_unknown')
        self.assertTrue(all(row['notAttempted'] for row in report['cells'][1:]))

    def test_workflow_contract_invocation_is_bytecode_free(self):
        """test_workflow_contract_invocation_is_bytecode_free verifies workflow contract invocation is bytecode free using controlled fixtures."""
        workflow = pathlib.Path(__file__).resolve().parents[1] / '.github/workflows/pr-validation.yml'
        self.assertIn('python3 -B ci/test_probe_cv_print.py', workflow.read_text())

    def test_no_probe_rows_or_wrong_browser_mode_cannot_attest_instrumentation(self):
        """test_no_probe_rows_or_wrong_browser_mode_cannot_attest_instrumentation verifies no probe rows or wrong browser mode cannot attest instrumentation using controlled fixtures."""
        for output in (synthetic_output(), synthetic_output(True, 'google-chrome')):
            row = probe.evaluate_cell(probe.CELLS[2], result(output))
            self.assertEqual(row['status'], 'instrumentation_coverage_incomplete')

    def test_only_finite_metadata_and_fixed_test_identities_are_exported(self):
        """test_only_finite_metadata_and_fixed_test_identities_are_exported verifies only finite metadata and fixed test identities are exported using controlled fixtures."""
        rows = []
        private = 'private-page-canary https://private.invalid/?token=private-key-canary'
        test = 'TestBrowserPDFWaitsForFontsAndKeepsPagination'
        for text in (private, 'CV_PRINT_PROBE phase=private-phase-canary outcome=success',
                     'CV_PRINT_PROBE browser_mode=auto ' + private,
                     'CV browser metadata: candidate_name="private-candidate-canary" version=1.2.3.4',
                     'CV_PRINT_PROBE phase=print outcome=success phase_ms=99999 total_ms=1 remaining_ms=1 context=active browser_version=1.2.3.4'):
            rows.append({'Action': 'output', 'Package': probe.CV_PACKAGE, 'Test': test, 'Output': text})
        rows.append({'Action': 'pass', 'Package': probe.CV_PACKAGE, 'Test': private})
        payload = synthetic_output() + ('\n'.join(json.dumps(row) for row in rows) + '\n').encode()
        facts = probe.project(payload)
        self.assertNotIn('canary', json.dumps(facts))
        self.assertEqual(facts['observations'], [])
        self.assertEqual(facts['invalidProbeLines'], 4)
        for text, kind in (
            ('CV browser runtime: go=go1.27.2 os=linux arch=amd64', 'runtime'),
            ('CV browser metadata: candidate_name="chromium" version=unavailable', 'browser_metadata'),
            ('CV browser metadata: candidate=unavailable version=unavailable', 'browser_metadata'),
        ):
            self.assertEqual(probe.observe(text, test)['kind'], kind)
        self.assertEqual(probe.runner_metadata({'ImageOS': private, 'ImageVersion': private}),
                         {'ImageOS': 'invalid_omitted', 'ImageVersion': 'invalid_omitted'})

    def test_missing_cache_is_a_prerequisite_failure_not_network_fallback(self):
        """test_missing_cache_is_a_prerequisite_failure_not_network_fallback verifies missing cache is a prerequisite failure not network fallback using controlled fixtures."""
        row = probe.evaluate_cell(probe.CELLS[0], result(b'', exit=1,
            stderr=b'module lookup disabled by GOPROXY=off private-module-canary'))
        self.assertEqual(row['status'], 'offline_dependency_prerequisite_missing')
        self.assertNotIn('canary', json.dumps(row))

    def test_invalid_json_package_completion_or_unexpected_failure_is_not_success(self):
        """test_invalid_json_package_completion_or_unexpected_failure_is_not_success verifies invalid json package completion or unexpected failure is not success using controlled fixtures."""
        row = probe.evaluate_cell(probe.CELLS[0], result(synthetic_output() + b'not-json\n'))
        self.assertEqual(row['status'], 'invalid_or_incomplete_output')
        unexpected = {'Action': 'fail', 'Package': probe.CV_PACKAGE, 'Test': 'UnknownPrivateCanary'}
        row = probe.evaluate_cell(probe.CELLS[0], result(synthetic_output() + json.dumps(unexpected).encode()))
        self.assertEqual(row['status'], 'test_failure')
        self.assertNotIn('UnknownPrivateCanary', json.dumps(row))

    def test_expected_pr_tree_and_module_bindings_are_checked_before_worktrees(self):
        """test_expected_pr_tree_and_module_bindings_are_checked_before_worktrees verifies expected pr tree and module bindings are checked before worktrees using controlled fixtures."""
        def fake_git(root, *args):
            """fake_git returns fixed Git metadata for prerequisite checks and rejects unexpected commands."""
            if args == ('status', '--porcelain'):
                return ''
            expression = args[-1]
            if ':' in expression:
                return probe.MODULE_BLOBS[expression.split(':', 1)[1]]
            if expression == 'HEAD':
                return '1' * 40
            if expression == 'HEAD^{tree}':
                return '2' * 40
            if expression == 'a' * 40 + '^{commit}':
                return 'a' * 40
            if expression == 'a' * 40 + '^{tree}':
                return '3' * 40
            raise AssertionError('unexpected command before prerequisite failure')
        with mock.patch.object(probe, 'git', side_effect=fake_git):
            with self.assertRaisesRegex(probe.ProbeError, 'checkout_tree_differs_from_expected_pr_head'):
                with probe.checkouts_for_ci(pathlib.Path('/owned'), 'a' * 40):
                    self.fail('different tree must not execute cells')
        with mock.patch.object(probe, 'git', return_value='private-module-canary'):
            with self.assertRaises(probe.ProbeError):
                with probe.checkouts_for_ci(pathlib.Path('/owned')):
                    self.fail('non-exact modules must not execute cells')


if __name__ == '__main__':
    unittest.main()
