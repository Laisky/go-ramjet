"""Pure driver contracts: no Go, browser, GitHub API, dependency install or network."""

import contextlib
import importlib.util
import io
import json
import pathlib
import signal
import subprocess
import sys
import tempfile
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


def shared_crawler_policy():
    """shared_crawler_policy binds the inherited crawler contract and the exact reviewed worker pin."""
    policy = probe.reviewed_transition()
    policy.pop('additionalTransitions', None)
    policy['schemaVersion'] = 2
    baseline = ['v1.0.2-0.20261011035106-d58d6c75a705',
                'h1:qQl7/KNGskIiioeWMvXBhTShXVEATElnuEYJqecd3gY=',
                'h1:ObKqjd9M67C/0VJnJxuWtbzewBzz/J5tH6YWs4gy/YE=']
    policy['sharedCrawler'] = {'module': 'github.com/Laisky/laisky-blog-graphql',
        'accepted': baseline, 'exact': list(baseline),
        'current': ['v1.0.2-0.20261021132434-f1aa1b9483de',
                    'h1:unDEfbTPz26ScuLl7Pu4gHEK0JOf3fAR9nzALdX7+8g=',
                    'h1:9z+qfn+yAvmkNtl++OBECSA1bRBa0UZciPtKAVJV9DE=']}
    return policy


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


    def test_dirty_metadata_names_only_verified_public_sources(self):
        """test_dirty_metadata_names_only_verified_public_sources excludes sensitive and arbitrary untracked names."""
        tracked = (b'internal/tasks/cv/pdf.go\0ci/probe_cv_print.py\0'
                   b'.github/instructions/laisky.instructions.md\0internal/private-key-canary.go\0')
        status = (b' M internal/tasks/cv/pdf.go\0?? ci/__pycache__/probe_cv_print.cpython-312.pyc\0'
                  b'?? ci/__pycache__/private-name-canary.cpython-312.pyc\0'
                  b' M .github/instructions/laisky.instructions.md\0'
                  b' M internal/private-key-canary.go\0?? arbitrary-name-canary.secret-extension-canary\0')
        metadata = probe.dirty_metadata(status, tracked)
        encoded = json.dumps(metadata)
        self.assertEqual(metadata['totalEntries'], 6)
        self.assertEqual(metadata['entries'][0]['path'], 'internal/tasks/cv/pdf.go')
        self.assertEqual(metadata['entries'][1]['trackedSourcePath'], 'ci/probe_cv_print.py')
        self.assertNotIn('path', metadata['entries'][1])
        self.assertEqual(metadata['entries'][1]['extension'], '.pyc')
        self.assertNotIn('trackedSourcePath', metadata['entries'][2])
        for private in ('private-name-canary', 'private-key-canary', 'arbitrary-name-canary',
                        'secret-extension-canary', 'laisky.instructions.md'):
            self.assertNotIn(private, encoded)
        self.assertFalse(metadata['contentsRead'])

    def test_dirty_metadata_cap_counts_all_entries_without_exporting_names(self):
        """test_dirty_metadata_cap_counts_all_entries_without_exporting_names retains totals beyond the 32-row cap."""
        status = b''.join(b'?? arbitrary-private-canary-' + str(index).encode() + b'.log\0' for index in range(70))
        metadata = probe.dirty_metadata(status, b'')
        self.assertEqual(metadata['totalEntries'], 70)
        self.assertEqual(metadata['statusCounts'], {'??': 70})
        self.assertEqual(metadata['categoryCounts'], {'untracked_other': 70})
        self.assertEqual(len(metadata['entries']), 32)
        self.assertTrue(metadata['entryLimitReached'])
        self.assertNotIn('canary', json.dumps(metadata))

    def test_dirty_guard_reads_only_status_and_tracked_name_metadata(self):
        """test_dirty_guard_reads_only_status_and_tracked_name_metadata keeps dirty state fatal without reading contents."""
        replies = [b'?? ci/__pycache__/probe_cv_print.cpython-312.pyc\0', b'ci/probe_cv_print.py\0']
        with mock.patch.object(probe, 'git_bytes', side_effect=replies) as git_bytes:
            with self.assertRaises(probe.ProbeError) as caught:
                probe.require_clean(pathlib.Path('/owned'), 'current_checkout_not_clean')
        self.assertEqual(str(caught.exception), 'current_checkout_not_clean')
        self.assertEqual(caught.exception.dirty_metadata['totalEntries'], 1)
        self.assertEqual(git_bytes.call_args_list, [
            mock.call(pathlib.Path('/owned'), 'status', '--porcelain=v1', '-z', '--untracked-files=all'),
            mock.call(pathlib.Path('/owned'), 'ls-files', '--cached', '-z'),
        ])

    def test_dirty_metadata_consumes_rename_sources_and_bounds_malformed_records(self):
        """test_dirty_metadata_consumes_rename_sources_and_bounds_malformed_records preserves counts without exposing old names."""
        tracked = b'ci/probe_cv_print.py\0internal/tasks/cv/pdf.go\0'
        status = (b'R  ci/probe_cv_print.py\0private-old-rename-canary\0'
                  b' M internal/tasks/cv/pdf.go\0BAD malformed-private-canary\0'
                  b'R  ci/new-private-canary.py\0')
        metadata = probe.dirty_metadata(status, tracked)
        self.assertEqual(metadata['totalEntries'], 4)
        self.assertEqual(metadata['statusCounts'], {'R ': 2, ' M': 1, 'unknown': 1})
        self.assertEqual(metadata['malformedRecords'], 2)
        self.assertEqual(metadata['entries'][0]['path'], 'ci/probe_cv_print.py')
        self.assertEqual(metadata['entries'][1]['path'], 'internal/tasks/cv/pdf.go')
        self.assertEqual(metadata['entries'][2]['status'], 'unknown')
        self.assertNotIn('canary', json.dumps(metadata))
        self.assertFalse(metadata['contentsRead'])

    def test_main_persists_dirty_artifact_and_fails_before_browser_execution(self):
        """test_main_persists_dirty_artifact_and_fails_before_browser_execution checks real JSON serialization and fatal wiring."""
        status = (b'?? ci/__pycache__/browser_sentry_build_args.cpython-312.pyc\0'
                  b'?? untracked-private-artifact-canary.key\0')
        tracked = b'ci/browser_sentry_build_args.py\0'
        original_save = probe.save_report
        with tempfile.TemporaryDirectory(prefix='probe-driver-artifact-contract-') as temporary:
            artifact = pathlib.Path(temporary) / 'report.json'
            def save(report):
                """save writes the real sanitized report to this contract's owned temporary artifact."""
                original_save(report, target=artifact)
            with mock.patch.object(probe, 'git_bytes', side_effect=[status, tracked]) as git_bytes, \
                    mock.patch.object(probe, 'save_report', side_effect=save) as save_mock, \
                    mock.patch.object(probe, 'run_matrix') as matrix, \
                    mock.patch.object(probe, 'run_browser') as browser, \
                    contextlib.redirect_stdout(io.StringIO()) as printed:
                exit_code = probe.main()
            report = json.loads(artifact.read_text())
        self.assertEqual(exit_code, 1)
        self.assertEqual(save_mock.call_count, 2)
        self.assertEqual(git_bytes.call_count, 2)
        matrix.assert_not_called()
        browser.assert_not_called()
        self.assertFalse(report['complete'])
        self.assertFalse(report['allCellsPassed'])
        self.assertEqual(report['cells'], [])
        self.assertEqual(report['status'], 'setup_or_cleanup_failure')
        self.assertEqual(report['marker'], 'current_checkout_not_clean')
        self.assertEqual(report['dirtyCheckout']['totalEntries'], 2)
        self.assertEqual(report['dirtyCheckout']['entries'][0]['trackedSourcePath'],
                         'ci/browser_sentry_build_args.py')
        self.assertFalse(report['rawOutputPersisted'])
        self.assertNotIn('canary', json.dumps(report) + printed.getvalue())
        self.assertNotIn('CV_PRINT_PROBE_CELL ', printed.getvalue())
        self.assertIn('CV_PRINT_PROBE_RESULT ', printed.getvalue())

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

    def test_expected_pr_tree_binding_is_checked_before_worktrees(self):
        """test_expected_pr_tree_binding_is_checked_before_worktrees verifies expected PR tree before creating worktrees using controlled fixtures."""
        def fake_git(root, *args):
            """fake_git returns fixed Git metadata for prerequisite checks and rejects unexpected commands."""
            if args == ('status', '--porcelain'):
                return ''
            expression = args[-1]
            if expression == 'HEAD':
                return '1' * 40
            if expression == 'HEAD^{tree}':
                return '2' * 40
            if expression == 'a' * 40 + '^{commit}':
                return 'a' * 40
            if expression == 'a' * 40 + '^{tree}':
                return '3' * 40
            raise AssertionError('unexpected command before prerequisite failure')
        with mock.patch.object(probe, 'git', side_effect=fake_git), mock.patch.object(probe, 'require_clean'):
            with self.assertRaisesRegex(probe.ProbeError, 'checkout_tree_differs_from_expected_pr_head'):
                with probe.checkouts_for_ci(pathlib.Path('/owned'), 'a' * 40):
                    self.fail('different tree must not execute cells')
        with mock.patch.object(probe, 'git', return_value='private-module-canary'), mock.patch.object(probe, 'require_clean'):
            with self.assertRaises(probe.ProbeError):
                with probe.checkouts_for_ci(pathlib.Path('/owned')):
                    self.fail('non-exact modules must not execute cells')


    def test_otlp_only_module_addition_keeps_browser_closure(self):
        """test_otlp_only_module_addition_keeps_browser_closure permits unrelated OTLP metadata additions."""
        output = 'github.com/chromedp/chromedp v0.20.1\n'
        sums = ('github.com/chromedp/chromedp v0.20.1 h1:browser\n'
                'github.com/chromedp/chromedp v0.20.1/go.mod h1:browsermod\n')
        baseline = probe.dependency_graph(output, sums)
        current = probe.dependency_graph(output, sums +
            'go.opentelemetry.io/otel/sdk/log v1.47.0 h1:logs\n'
            'go.opentelemetry.io/otel/sdk/log v1.47.0/go.mod h1:logsmod\n')
        probe.require_dependency_match({'accepted': baseline, 'exact': baseline, 'current': current})
        self.assertEqual(baseline, current)

    def test_browser_control_mutation_or_replacement_is_rejected(self):
        """test_browser_control_mutation_or_replacement_is_rejected preserves relevant control invariants."""
        output = 'github.com/chromedp/chromedp v0.20.1\n'
        sums = ('github.com/chromedp/chromedp v0.20.1 h1:browser\n'
                'github.com/chromedp/chromedp v0.20.1/go.mod h1:browsermod\n')
        baseline = probe.dependency_graph(output, sums)
        variants = (
            probe.dependency_graph(output.replace('v0.20.1', 'v0.21.0'), sums.replace('v0.20.1', 'v0.21.0')),
            probe.dependency_graph(output, sums.replace('h1:browsermod', 'h1:changedmod')),
            probe.dependency_graph(output, sums.replace('h1:browser\n', 'h1:changed\n')),
        )
        for current in variants:
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_graph_differs'):
                probe.require_dependency_match({'accepted': baseline, 'exact': baseline, 'current': current})
        with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_replaced_or_invalid'):
            probe.dependency_graph(output.rstrip() + ' replaced\n', sums)
        with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_checksum_missing'):
            probe.dependency_graph(output, '')

    def test_pdf_assertion_dependency_drift_is_bound_by_resolver(self):
        """test_pdf_assertion_dependency_drift_is_bound_by_resolver rejects changed test-only PDF dependencies."""
        browser = 'github.com/chromedp/chromedp v0.20.1\n'
        pdf = 'github.com/pdfcpu/pdfcpu v0.16.1\n'
        sums = ('github.com/chromedp/chromedp v0.20.1 h1:browser\n'
                'github.com/chromedp/chromedp v0.20.1/go.mod h1:browsermod\n'
                'github.com/pdfcpu/pdfcpu v0.16.1 h1:pdf\n'
                'github.com/pdfcpu/pdfcpu v0.16.1/go.mod h1:pdfmod\n')
        def selected(args, **kwargs):
            """selected models go list selecting PDF only when its actual test dependency root is supplied."""
            output = browser + (pdf if 'github.com/pdfcpu/pdfcpu/pkg/api' in args else '')
            return subprocess.CompletedProcess(args, 0, output.encode(), b'')
        with tempfile.TemporaryDirectory(prefix='probe-pdf-dependency-contract-') as temporary:
            root = pathlib.Path(temporary)
            (root / 'go.sum').write_text(sums)
            with mock.patch.object(probe.subprocess, 'run', side_effect=selected):
                baseline = probe.dependency_snapshot(root)
                (root / 'go.sum').write_text(sums.replace('h1:pdf\n', 'h1:changedpdf\n'))
                changed = probe.dependency_snapshot(root)
        with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_graph_differs'):
            probe.require_dependency_match({'accepted': baseline, 'exact': baseline, 'current': changed})

    def test_dependency_snapshot_is_offline_readonly_and_hides_prerequisite_errors(self):
        """test_dependency_snapshot_is_offline_readonly_and_hides_prerequisite_errors preserves cache-only failures."""
        failure = subprocess.CompletedProcess([], 1, b'', b'private-module-canary')
        with mock.patch.object(probe.subprocess, 'run', return_value=failure) as execute:
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_graph_unavailable') as caught:
                probe.dependency_snapshot(pathlib.Path('/owned'))
        self.assertNotIn('canary', str(caught.exception))
        self.assertIn('-mod=readonly', execute.call_args.args[0])
        self.assertEqual(execute.call_args.kwargs['env']['GOPROXY'], 'off')
        self.assertEqual(execute.call_args.kwargs['env']['GOSUMDB'], 'off')
        self.assertEqual(execute.call_args.kwargs['env']['GONOPROXY'], 'none')
        self.assertEqual(execute.call_args.kwargs['env']['GOWORK'], 'off')



class ReviewedSDKTransitionContracts(unittest.TestCase):
    """ReviewedSDKTransitionContracts model exact dependency and cache boundaries without Go/network."""

    def snapshots(self, policy):
        """snapshots returns immutable inherited and reviewed candidate module profiles."""
        other = ('github.com/chromedp/chromedp', 'v0.20.1', 'h1:browser', 'h1:browsermod')
        profiles = (policy,) + ((policy['sharedCrawler'],) if 'sharedCrawler' in policy else ())
        return {role: tuple(sorted([(profile['module'], *profile[role]) for profile in profiles] + [other]))
                for role in ('accepted', 'exact', 'current')}

    def test_reviewed_shared_crawler_pin_preserves_historical_graph(self):
        """test_reviewed_shared_crawler_pin_preserves_historical_graph accepts the two exact reviewed changes."""
        policy = shared_crawler_policy()
        snapshots = self.snapshots(policy)
        probe.require_dependency_match(snapshots, policy)
        self.assertEqual(snapshots['accepted'], snapshots['exact'])
        self.assertNotEqual(snapshots['accepted'], snapshots['current'])
        self.assertEqual(len(probe.CELLS), 6)

    def test_exact_transition_keeps_historical_profiles_and_six_cells(self):
        """test_exact_transition_keeps_historical_profiles_and_six_cells permits only the reviewed change."""
        policy = shared_crawler_policy()
        snapshots = self.snapshots(policy)
        probe.require_dependency_match(snapshots, policy)
        self.assertNotEqual(snapshots['accepted'], snapshots['current'])
        self.assertEqual(snapshots['accepted'], snapshots['exact'])
        self.assertEqual(len(probe.CELLS), 6)
        self.assertEqual([row[1] for row in probe.CELLS], ['accepted', 'exact', 'current', 'current', 'current', 'current'])

    def test_unreviewed_versions_checksums_missing_or_duplicate_profiles_fail(self):
        """test_unreviewed_versions_checksums_missing_or_duplicate_profiles_fail binds both modules in every role."""
        policy = shared_crawler_policy()
        for profile, marker in ((policy, 'browser_sdk_profile_differs'),
                                (policy['sharedCrawler'], 'browser_crawler_profile_differs')):
            for role in ('accepted', 'exact', 'current'):
                for change in (1, 2, 3, 'missing', 'duplicate'):
                    with self.subTest(module=profile['module'], role=role, change=change):
                        snapshots = self.snapshots(policy)
                        changed = list(snapshots[role])
                        index = next(i for i, row in enumerate(changed) if row[0] == profile['module'])
                        if change == 'missing':
                            changed.pop(index)
                        elif change == 'duplicate':
                            changed.append(changed[index])
                        else:
                            values = list(changed[index])
                            values[change] = 'unreviewed-value'
                            changed[index] = tuple(values)
                        snapshots[role] = tuple(changed)
                        with self.assertRaisesRegex(probe.ProbeError, marker):
                            probe.require_dependency_match(snapshots, policy)

    def test_manifest_preserves_v1_and_rejects_unreviewed_companions(self):
        """test_manifest_preserves_v1_and_rejects_unreviewed_companions keeps the manifest exact and source-owned."""
        policy = shared_crawler_policy()
        self.assertEqual(probe.reviewed_transition()['sharedCrawler'], policy['sharedCrawler'])
        legacy = {key: value for key, value in policy.items() if key != 'sharedCrawler'}
        legacy['schemaVersion'] = 1
        with tempfile.TemporaryDirectory(prefix='browser-reviewed-policy-') as temporary:
            path = pathlib.Path(temporary) / 'policy.json'
            path.write_text(json.dumps(legacy))
            self.assertEqual(probe.reviewed_transition(path), legacy)
            probe.require_dependency_match(self.snapshots(legacy), legacy)
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_graph_differs'):
                probe.require_dependency_match(self.snapshots(policy), legacy)
            mutations = ((('schemaVersion',), 3), (('unknown',), []),
                (('sharedCrawler',), None),
                (('sharedCrawler', 'module'), 'example.test/unreviewed'), (('sharedCrawler', 'unknown'), []),
                (('sharedCrawler', 'accepted', 0), 'latest'), (('sharedCrawler', 'accepted', 1), 'h1:invalid'),
                (('sharedCrawler', 'exact'), policy['sharedCrawler']['current']),
                (('sharedCrawler', 'current'), policy['sharedCrawler']['accepted']))
            for keys, value in mutations:
                changed = json.loads(json.dumps(policy))
                target = changed
                for key in keys[:-1]:
                    target = target[key]
                target[keys[-1]] = value
                path.write_text(json.dumps(changed))
                with self.subTest(keys=keys), self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_policy_invalid'):
                    probe.reviewed_transition(path)
            for field in ('schemaVersion', 'module'):
                path.write_text(json.dumps(policy).replace('"' + field + '":', '"' + field + '": null, "' + field + '":'))
                with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_policy_invalid'):
                    probe.reviewed_transition(path)

    def test_other_module_and_root_changes_fail(self):
        """test_other_module_and_root_changes_fail prevents an SDK transition from exempting other controls."""
        policy = shared_crawler_policy()
        for column in (0, 1, 2, 3):
            snapshots = self.snapshots(policy)
            changed = []
            for row in snapshots['current']:
                values = list(row)
                if row[0] == 'github.com/chromedp/chromedp':
                    values[column] = 'unreviewed-other'
                changed.append(tuple(values))
            snapshots['current'] = tuple(changed)
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_graph_differs'):
                probe.require_dependency_match(snapshots, policy)
        with mock.patch.object(probe, 'CONTROL_IMPORTS', (*probe.CONTROL_IMPORTS, 'example.test/unreviewed/root')):
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_roots_differ'):
                probe.reviewed_transition()

    @contextlib.contextmanager
    def cached_module(self):
        """cached_module supplies an owned synthetic cache without pretending the zip is a real Go archive."""
        with tempfile.TemporaryDirectory(prefix='browser-module-cache-contract-') as temporary:
            root = pathlib.Path(temporary)
            directory = root / 'module'
            directory.mkdir()
            modfile = root / 'v1.0.0.mod'
            modfile.write_bytes(b'module example.test/module\n')
            checksum = 'h1:synthetic-zip'
            line = probe.hashlib.sha256(modfile.read_bytes()).hexdigest() + '  go.mod\n'
            modsum = 'h1:' + probe.base64.b64encode(probe.hashlib.sha256(line.encode()).digest()).decode()
            modfile.with_suffix('.zip').write_bytes(b'synthetic archive; actual verification is mocked')
            modfile.with_suffix('.ziphash').write_text(checksum)
            entry = {'Path': 'example.test/module', 'Version': 'v1.0.0', 'Sum': checksum,
                     'GoModSum': modsum, 'Dir': str(directory), 'GoMod': str(modfile)}
            yield root, entry, ((entry['Path'], entry['Version'], checksum, modsum),)

    def execute_cache(self, root, entry, graph, verify_exit=0):
        """execute_cache runs the real metadata checks with mocked Go commands and private stderr controls."""
        metadata = subprocess.CompletedProcess([], 0, json.dumps(entry).encode(), b'')
        verified = subprocess.CompletedProcess([], verify_exit, b'', b'private-cache-canary')
        with mock.patch.object(probe.subprocess, 'run', side_effect=[metadata, verified]) as execute:
            probe.verify_dependency_cache(root, graph)
        return execute

    def test_cache_presence_identity_and_readonly_verification(self):
        """test_cache_presence_identity_and_readonly_verification verifies the explicit offline cache boundary."""
        with self.cached_module() as (root, entry, graph):
            execute = self.execute_cache(root, entry, graph)
            self.assertEqual(execute.call_args_list[1].args[0], ['go', 'mod', 'verify'])
            for call in execute.call_args_list:
                self.assertEqual(call.kwargs['env']['GOPROXY'], 'off')
                self.assertEqual(call.kwargs['env']['GOSUMDB'], 'off')
                self.assertEqual(call.kwargs['env']['GOFLAGS'], '-mod=readonly')

    def test_missing_replaced_or_mismatched_cache_fails(self):
        """test_missing_replaced_or_mismatched_cache_fails rejects gaps before a browser can start."""
        for field in ('Version', 'Sum', 'GoModSum', 'Replace', 'Dir'):
            with self.subTest(field=field), self.cached_module() as (root, entry, graph):
                entry[field] = {'Path': 'example.test/replaced'} if field == 'Replace' else 'unreviewed-value'
                with self.assertRaises(probe.ProbeError):
                    self.execute_cache(root, entry, graph)
        for suffix in ('.zip', '.ziphash', '.mod'):
            with self.subTest(missing=suffix), self.cached_module() as (root, entry, graph):
                pathlib.Path(entry['GoMod']).with_suffix(suffix).unlink()
                with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_cache_incomplete'):
                    self.execute_cache(root, entry, graph)

    def test_cache_bytes_or_verify_failure_are_visible_without_raw_errors(self):
        """test_cache_bytes_or_verify_failure_are_visible_without_raw_errors covers tamper and verification failure."""
        for suffix in ('.ziphash', '.mod'):
            with self.subTest(tampered=suffix), self.cached_module() as (root, entry, graph):
                pathlib.Path(entry['GoMod']).with_suffix(suffix).write_text('changed-cache')
                with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_cache_identity_differs'):
                    self.execute_cache(root, entry, graph)
        with self.cached_module() as (root, entry, graph):
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_cache_verify_failed') as caught:
                self.execute_cache(root, entry, graph, verify_exit=1)
            self.assertNotIn('canary', str(caught.exception))

    def test_missing_cache_prerequisite_has_fixed_error(self):
        """test_missing_cache_prerequisite_has_fixed_error does not expose raw Go diagnostics."""
        failure = subprocess.CompletedProcess([], 1, b'', b'private-cache-canary')
        with mock.patch.object(probe.subprocess, 'run', return_value=failure):
            with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_cache_unavailable') as caught:
                probe.verify_dependency_cache(pathlib.Path('/owned'), (('example.test/module', 'v1', 'h1:x', 'h1:y'),))
        self.assertNotIn('canary', str(caught.exception))



class CachePreparationContracts(unittest.TestCase):
    """CachePreparationContracts verify the explicit preparation boundary without fetching or running Go."""

    @contextlib.contextmanager
    def profile(self, role='accepted', policy=None):
        """profile creates only reviewed public module bytes in an owned temporary directory."""
        policy = policy or shared_crawler_policy()
        with tempfile.TemporaryDirectory(prefix='browser-preparation-contract-') as temporary:
            root = pathlib.Path(temporary)
            profiles = (policy,) + ((policy['sharedCrawler'],) if 'sharedCrawler' in policy else ())
            (root / 'go.mod').write_text('module example.test/fixture\n\ngo 1.27.0\n\n' + ''.join(
                'require ' + profile['module'] + ' ' + profile[role][0] + '\n' for profile in profiles))
            (root / 'go.sum').write_text(''.join(profile['module'] + ' ' + profile[role][0] + suffix + ' ' + checksum + '\n'
                for profile in profiles for suffix, checksum in (('', profile[role][1]), ('/go.mod', profile[role][2]))))
            yield root, policy

    def test_preparation_warms_exact_historical_and_current_crawler_cache(self):
        """test_preparation_warms_exact_historical_and_current_crawler_cache requires both pinned modules."""
        policy = shared_crawler_policy()
        for role in ('accepted', 'exact', 'current'):
            with self.subTest(role=role), self.profile(role, policy) as (root, _):
                profiles = (policy, policy['sharedCrawler'])
                wanted = {profile['module'] + '@' + profile[role][0]: profile for profile in profiles}
                def download(args, **kwargs):
                    """download models an initially absent cache entry becoming present only for the requested pin."""
                    profile = wanted[args[-1]]
                    return subprocess.CompletedProcess(args, 0, json.dumps(dict(zip(
                        ('Path', 'Version', 'Sum', 'GoModSum'), (profile['module'], *profile[role])))).encode(), b'')
                with mock.patch.object(probe.subprocess, 'run', side_effect=download) as execute:
                    result = probe.prepare_dependency_cache(root, role, policy)
                self.assertEqual({call.args[0][-1] for call in execute.call_args_list}, set(wanted))
                self.assertEqual(result['sdkProfile'], policy[role])
                self.assertEqual(result['sharedCrawlerProfile'], policy['sharedCrawler'][role])
                self.assertEqual(result['status'], 'cache_prepared_not_qualified')

    def test_legacy_policy_preparation_preserves_sdk_only_report(self):
        """test_legacy_policy_preparation_preserves_sdk_only_report retains the v1 preparation contract."""
        policy = shared_crawler_policy()
        policy.pop('sharedCrawler')
        policy['schemaVersion'] = 1
        with self.profile(policy=policy) as (root, _):
            metadata = dict(zip(('Path', 'Version', 'Sum', 'GoModSum'), (policy['module'], *policy['accepted'])))
            with mock.patch.object(probe.subprocess, 'run', return_value=subprocess.CompletedProcess(
                    [], 0, json.dumps(metadata).encode(), b'')) as execute:
                result = probe.prepare_dependency_cache(root, 'accepted', policy)
            self.assertEqual(execute.call_count, 1)
            self.assertEqual(execute.call_args.args[0][-1], policy['module'] + '@' + policy['accepted'][0])
            self.assertEqual(result['sdkProfile'], policy['accepted'])
            self.assertNotIn('sharedCrawlerProfile', result)

    def test_explicit_preparation_uses_bound_copies_and_public_checked_proxy(self):
        """test_explicit_preparation_uses_bound_copies_and_public_checked_proxy preserves original manifests."""
        for role in ('accepted', 'exact', 'current'):
            with self.subTest(role=role), self.profile(role) as (root, policy):
                before = [(root / name).read_bytes() for name in ('go.mod', 'go.sum')]
                def download(args, **kwargs):
                    """download checks actual preparation arguments and copied input bytes before returning success."""
                    profile = next(profile for profile in (policy, policy['sharedCrawler']) if args[-1].startswith(profile['module'] + '@'))
                    self.assertEqual(args, ['go', 'mod', 'download', '-json', profile['module'] + '@' + profile[role][0]])
                    self.assertNotEqual(kwargs['cwd'], root)
                    self.assertEqual([(kwargs['cwd'] / name).read_bytes() for name in ('go.mod', 'go.sum')], before)
                    self.assertEqual(kwargs['timeout'], 180)
                    env = kwargs['env']
                    self.assertEqual(env['GOPROXY'], 'https://proxy.golang.org')
                    self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
                    self.assertEqual(env['GOAUTH'], 'off')
                    self.assertEqual(env['GONOPROXY'], 'none')
                    self.assertEqual(env['GONOSUMDB'], 'none')
                    self.assertEqual(env['GOWORK'], 'off')
                    self.assertEqual(env['GOFLAGS'], '-mod=readonly')
                    return subprocess.CompletedProcess(args, 0, json.dumps(dict(zip(
                        ('Path', 'Version', 'Sum', 'GoModSum'), (profile['module'], *profile[role])))).encode(), b'')
                with mock.patch.object(probe.subprocess, 'run', side_effect=download):
                    result = probe.prepare_dependency_cache(root, role, policy)
                self.assertEqual(result['status'], 'cache_prepared_not_qualified')
                self.assertEqual([(root / name).read_bytes() for name in ('go.mod', 'go.sum')], before)

    def test_missing_mismatched_or_replaced_profile_never_fetches(self):
        """test_missing_mismatched_or_replaced_profile_never_fetches rejects unreviewed inputs before Go starts."""
        for mutation in ('version', 'checksum', 'missing checksum', 'replacement'):
            with self.subTest(mutation=mutation), self.profile() as (root, policy):
                if mutation == 'version':
                    path = root / 'go.mod'
                    path.write_text(path.read_text().replace(policy['accepted'][0], 'v6.9.9'))
                elif mutation == 'checksum':
                    path = root / 'go.sum'
                    path.write_text(path.read_text().replace(policy['accepted'][1], 'h1:unreviewed'))
                elif mutation == 'missing checksum':
                    (root / 'go.sum').write_text('')
                else:
                    path = root / 'go.mod'
                    path.write_text(path.read_text() + '\nreplace example.test/unreviewed => ./private\n')
                with mock.patch.object(probe.subprocess, 'run') as execute:
                    with self.assertRaises(probe.ProbeError):
                        probe.prepare_dependency_cache(root, 'accepted', policy)
                execute.assert_not_called()

    def test_invalid_crawler_manifest_prevents_all_cache_downloads(self):
        """test_invalid_crawler_manifest_prevents_all_cache_downloads validates the companion before preparation."""
        for mutation in ('version', 'checksum', 'missing checksum'):
            with self.subTest(mutation=mutation), self.profile() as (root, policy):
                companion = policy['sharedCrawler']
                if mutation == 'version':
                    path = root / 'go.mod'
                    path.write_text(path.read_text().replace(companion['accepted'][0], 'v1.9.9'))
                else:
                    path = root / 'go.sum'
                    contents = path.read_text()
                    path.write_text(contents.replace(companion['accepted'][1], 'h1:unreviewed') if mutation == 'checksum'
                                    else '\n'.join(line for line in contents.splitlines() if not line.startswith(companion['module'])))
                with mock.patch.object(probe.subprocess, 'run') as execute, self.assertRaises(probe.ProbeError):
                    probe.prepare_dependency_cache(root, 'accepted', policy)
                execute.assert_not_called()

    def test_sdk_cache_success_cannot_mask_crawler_preparation_failure(self):
        """test_sdk_cache_success_cannot_mask_crawler_preparation_failure fails closed after the SDK is prepared."""
        for outcome in ('failure', 'timeout', 'Path', 'Version', 'Sum', 'GoModSum', 'Error', 'invalid JSON', 'changed copy'):
            with self.subTest(outcome=outcome), self.profile() as (root, policy):
                def download(args, **kwargs):
                    """download succeeds for the SDK and then models an independently failing crawler download."""
                    companion = args[-1].startswith(policy['sharedCrawler']['module'] + '@')
                    if companion and outcome == 'timeout':
                        raise subprocess.TimeoutExpired(args, 180, stderr=b'private-companion-canary')
                    profile = policy['sharedCrawler'] if companion else policy
                    metadata = dict(zip(('Path', 'Version', 'Sum', 'GoModSum'), (profile['module'], *profile['accepted'])))
                    if companion and outcome in ('Path', 'Version', 'Sum', 'GoModSum', 'Error'):
                        metadata[outcome] = 'unreviewed-value'
                    if companion and outcome == 'changed copy':
                        (kwargs['cwd'] / 'go.sum').write_text('unreviewed')
                    output = b'not-json' if companion and outcome == 'invalid JSON' else json.dumps(metadata).encode()
                    return subprocess.CompletedProcess(args, int(companion and outcome == 'failure'), output, b'private-companion-canary')
                with mock.patch.object(probe.subprocess, 'run', side_effect=download) as execute:
                    with self.assertRaises(probe.ProbeError) as caught:
                        probe.prepare_dependency_cache(root, 'accepted', policy)
                self.assertEqual(execute.call_count, 2)
                self.assertNotIn('canary', str(caught.exception))

    def test_preparation_errors_or_manifest_changes_fail_visibly(self):
        """test_preparation_errors_or_manifest_changes_fail_visibly never treat download failure as acceptance."""
        for outcome in ('failure', 'timeout', 'changed copy'):
            with self.subTest(outcome=outcome), self.profile() as (root, policy):
                def download(args, **kwargs):
                    """download models each failure without contacting a proxy."""
                    if outcome == 'timeout':
                        raise subprocess.TimeoutExpired(args, 180, stderr=b'private-preparation-canary')
                    if outcome == 'changed copy':
                        (kwargs['cwd'] / 'go.sum').write_text('changed unreviewed manifest')
                    return subprocess.CompletedProcess(args, 1 if outcome == 'failure' else 0,
                                                       b'', b'private-preparation-canary')
                with mock.patch.object(probe.subprocess, 'run', side_effect=download):
                    with self.assertRaises(probe.ProbeError) as caught:
                        probe.prepare_dependency_cache(root, 'accepted', policy)
                self.assertNotIn('canary', str(caught.exception))

    def test_preparation_rejects_unconfirmed_download_metadata(self):
        """test_preparation_rejects_unconfirmed_download_metadata requires checksum-confirmed exact SDK output."""
        for field in ('Path', 'Version', 'Sum', 'GoModSum', 'Error', 'invalid JSON'):
            with self.subTest(field=field), self.profile() as (root, policy):
                result = dict(zip(('Path', 'Version', 'Sum', 'GoModSum'), (policy['module'], *policy['accepted'])))
                result[field] = 'unreviewed-download'
                output = b'not-json' if field == 'invalid JSON' else json.dumps(result).encode()
                with mock.patch.object(probe.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, output, b'')):
                    with self.assertRaises(probe.ProbeError):
                        probe.prepare_dependency_cache(root, 'accepted', policy)

    def test_preparation_report_is_not_browser_qualification(self):
        """test_preparation_report_is_not_browser_qualification keeps preparation separate from all six cells."""
        with tempfile.TemporaryDirectory(prefix='browser-preparation-report-') as temporary:
            target = pathlib.Path(temporary) / 'report.json'
            paths = {'preparation': [{'role': 'accepted', 'status': 'cache_prepared_not_qualified'}]}
            with mock.patch.object(probe, 'checkouts_for_ci', return_value=contextlib.nullcontext(paths)) as checkouts, \
                 mock.patch.object(probe, 'run_matrix') as matrix, \
                 mock.patch.object(probe, 'PREPARATION_REPORT_PATH', target), contextlib.redirect_stdout(io.StringIO()):
                result = probe.prepare_cache_report(pathlib.Path('/owned'), '1' * 40)
            self.assertEqual(result, 0)
            checkouts.assert_called_once_with(pathlib.Path('/owned'), '1' * 40, prepare_cache=True)
            matrix.assert_not_called()
            report = json.loads(target.read_text())
            self.assertFalse(report['qualified'])
            self.assertEqual(report['status'], 'cache_prepared_not_qualified')
            self.assertNotIn('allCellsPassed', report)


if __name__ == '__main__':
    from test_browser_transition_v3 import BrowserTransitionV3Contracts
    unittest.main()
