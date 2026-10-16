#!/usr/bin/env python3
"""Run six distinct synthetic browser controls; never replay a production fetch.

This module has no import side effects. Only the hosted CI entry point invokes
browsers. Raw Go test output stays in memory for hashes and finite projections.
"""

import contextlib
import datetime
import hashlib
import json
import math
import os
import pathlib
import re
import signal
import subprocess
import sys
import tempfile
import time

REPORT_PATH = pathlib.Path('/tmp/task02-cv-print-probe-report.json')
ACCEPTED = '3d2c5dc0cae3f26c1600f0f23701e2b1cc26f147'
EXACT = '2fd1f2586c2188935675851101626c36490435aa'
MODULE_BLOBS = {'go.mod': 'a756744ff35542a652e5eaf5aa3a5d908379627d',
                'go.sum': '2b5f1468f94138a6dbf0f108972a741d46397aa0'}
CV_PACKAGE = 'github.com/Laisky/go-ramjet/internal/tasks/cv'
TASK_PACKAGE = 'github.com/Laisky/go-ramjet/internal/tasks/gptchat/tasks'
PACKAGES = {CV_PACKAGE: 'cv', TASK_PACKAGE: 'renderer'}
SELECTOR = '^Test(Browser|Renderer(Egress|Browser))'
BROWSER_ENV = 'CV_PDF_PRINT_PROBE_BROWSER'
WATCHDOG_SECONDS = 240
CELLS = (
    ('accepted_auto_joint', 'accepted', 'auto', False, False),
    ('exact_auto_joint', 'exact', 'auto', False, False),
    ('probe_auto_joint', 'current', 'auto', False, True),
    ('probe_chrome_joint', 'current', 'google-chrome', False, True),
    ('probe_auto_p1', 'current', 'auto', True, True),
    ('probe_chrome_p1', 'current', 'google-chrome', True, True),
)
# These identities are fixed source-owned synthetic fixtures, including two
# intentional synthetic URL subtest names. No arbitrary runtime test text passes.
ORIGINAL_TESTS = frozenset((
    'TestBrowserDynamicHTMLPreservesHeadersAndScripts',
    'TestBrowserPDFRejectsFontPromiseFailure',
    'TestBrowserPDFWaitsForFontsAndKeepsPagination',
    'TestRendererBrowserCanonicalDocument',
    'TestRendererBrowserCanonicalDocument/http://RENDER.EXAMPLE.:80/',
    'TestRendererBrowserCanonicalDocument/http://render.example#fragment',
    'TestRendererBrowserDelayedBody', 'TestRendererBrowserHTTPS',
    'TestRendererBrowserNonProxyTCP', 'TestRendererBrowserNonProxyUDP',
    'TestRendererBrowserPrivateRequests', 'TestRendererBrowserPrivateRequests/iframe',
    'TestRendererBrowserPrivateRequests/image', 'TestRendererBrowserPrivateRequests/popup',
    'TestRendererBrowserPrivateRequests/preconnect', 'TestRendererBrowserPrivateRequests/redirect',
    'TestRendererBrowserPrivateRequests/websocket', 'TestRendererBrowserPrivateRequests/webtransport',
    'TestRendererBrowserPrivateRequests/worker', 'TestRendererBrowserPublicRedirectAndResource',
    'TestRendererBrowserReachableSentinelNegativeControl', 'TestRendererBrowserUDPReachableNegativeControl',
    'TestRendererEgressEncoding', 'TestRendererEgressEncoding/br', 'TestRendererEgressEncoding/deflate',
    'TestRendererEgressEncoding/gzip', 'TestRendererEgressEncoding/gzip-bomb',
    'TestRendererEgressEncoding/identity', 'TestRendererEgressIPv6', 'TestRendererEgressOriginCache',
    'TestRendererEgressPeerAndCancellation', 'TestRendererEgressPinned',
    'TestRendererEgressRedirects', 'TestRendererEgressRejectsBeforeDial', 'TestRendererEgressTLS',
))
CV_TESTS = frozenset(name for name in ORIGINAL_TESTS if name.startswith('TestBrowserPDF'))
VERSION = r'(?:[0-9]{1,5}\.){3}[0-9]{1,5}|unavailable'
PHASE = re.compile(r'CV_PRINT_PROBE phase=(startup|browser_version|navigation|body_ready|fonts|print) '
                   r'outcome=(begin|success|failure) phase_ms=([0-9]{1,5}) total_ms=([0-9]{1,5}) '
                   r'remaining_ms=([0-9]{1,5}) context=(active|canceled|deadline_exceeded) '
                   r'browser_version=(' + VERSION + r')$')
MODE = re.compile(r'CV_PRINT_PROBE browser_mode=(auto|google-chrome)$')
PARALLEL = re.compile(r'CV_PRINT_PROBE runtime_parallelism=([0-9]{1,5}) '
                      r'test_parallelism=([0-9]{1,5}) cpu_count=([0-9]{1,5})$')
METADATA = re.compile(r'CV browser metadata: candidate_name="(headless_shell|headless-shell|chromium|'
                      r'chromium-browser|google-chrome|google-chrome-stable|google-chrome-beta|'
                      r'google-chrome-unstable|chrome)" version=(' + VERSION + r')$')
RUNTIME = re.compile(r'CV browser runtime: go=(go[0-9]{1,3}\.[0-9]{1,3}(?:\.[0-9]{1,3})?) '
                     r'os=(linux|darwin|windows) arch=(amd64|arm64)$')


class ProbeError(RuntimeError):
    """A fixed marker; never attach raw subprocess output."""


def stamp():
    """stamp returns the current UTC timestamp as ISO 8601 text."""
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def digest(data):
    """digest returns the SHA256 hexadecimal fingerprint of supplied bytes."""
    return hashlib.sha256(data).hexdigest()


def require(value, marker):
    """require raises the fixed marker when the supplied condition is false."""
    if not value:
        raise ProbeError(marker)


def command_for(serial_packages):
    """command_for returns the original joint-test arguments with optional package serialization."""
    args = ['dbus-run-session', '--', 'go', 'test', '-json']
    if serial_packages:
        args.extend(['-p', '1'])
    return args + ['-tags=browser_integration', './internal/tasks/cv',
                   './internal/tasks/gptchat/tasks', '-run', SELECTOR, '-count=1']


def observe(output, test):
    """observe accepts fixed probe grammar and returns finite metadata or None without raw error text."""
    if test not in CV_TESTS or not isinstance(output, str):
        return None
    if output.rstrip('\r\n').endswith('CV browser metadata: candidate=unavailable version=unavailable'):
        return {'test': test, 'kind': 'browser_metadata', 'candidate': 'unavailable', 'version': 'unavailable'}
    for marker, pattern, kind in (
        ('CV_PRINT_PROBE phase=', PHASE, 'phase'),
        ('CV_PRINT_PROBE browser_mode=', MODE, 'browser_mode'),
        ('CV_PRINT_PROBE runtime_parallelism=', PARALLEL, 'parallelism'),
        ('CV browser metadata:', METADATA, 'browser_metadata'),
        ('CV browser runtime:', RUNTIME, 'runtime'),
    ):
        position = output.find(marker)
        if position < 0:
            continue
        match = pattern.fullmatch(output[position:].rstrip('\r\n'))
        if not match:
            return None
        values = match.groups()
        event = {'test': test, 'kind': kind}
        if kind == 'phase':
            numbers = [int(value) for value in values[2:5]]
            if any(value > 30000 for value in numbers):
                return None
            event.update(phase=values[0], outcome=values[1], phaseMS=numbers[0],
                         totalMS=numbers[1], remainingMS=numbers[2],
                         context=values[5], browserVersion=values[6])
        elif kind == 'parallelism':
            numbers = [int(value) for value in values]
            if any(value > 65536 for value in numbers):
                return None
            event.update(runtimeParallelism=numbers[0], testParallelism=numbers[1], cpuCount=numbers[2])
        elif kind == 'browser_mode':
            event['browserMode'] = values[0]
        elif kind == 'browser_metadata':
            event.update(candidate=values[0], version=values[1])
        else:
            event.update(go=values[0], os=values[1], arch=values[2])
        return event
    return None


def project(stdout):
    """project returns bounded test identities, package outcomes and safe observations from Go JSON bytes."""
    passed, terminal_packages = set(), set()
    tests, packages, observations = [], [], []
    invalid, invalid_probe, failures, skips, overflow = 0, 0, 0, 0, False
    for line in stdout.splitlines():
        try:
            row = json.loads(line)
        except (ValueError, UnicodeDecodeError):
            invalid += 1
            continue
        if not isinstance(row, dict):
            invalid += 1
            continue
        package, test, action = row.get('Package'), row.get('Test'), row.get('Action')
        if package not in PACKAGES:
            invalid += 1
            continue
        if action == 'fail':
            failures += 1
        if action == 'skip':
            skips += 1
        expected_package = CV_PACKAGE if test in CV_TESTS else TASK_PACKAGE
        if test in ORIGINAL_TESTS and package == expected_package and action in ('pass', 'fail', 'skip'):
            if action == 'pass':
                passed.add(test)
            if len(tests) < 256:
                tests.append({'test': test, 'package': PACKAGES[package], 'action': action})
            else:
                overflow = True
        elif test is None and action in ('pass', 'fail', 'skip'):
            if action == 'pass':
                terminal_packages.add(package)
            elapsed = row.get('Elapsed', 0)
            if not isinstance(elapsed, (int, float)) or not math.isfinite(elapsed) or not 0 <= elapsed <= 86400:
                invalid += 1
                elapsed = 0
            packages.append({'package': PACKAGES[package], 'action': action, 'durationMS': int(elapsed * 1000)})
        if action == 'output':
            output = row.get('Output')
            event = observe(output, test)
            if (event is None and test in CV_TESTS and isinstance(output, str) and
                    any(marker in output for marker in ('CV_PRINT_PROBE ', 'CV browser metadata:', 'CV browser runtime:'))):
                invalid_probe += 1
            if event is not None:
                if len(observations) < 256:
                    observations.append(event)
                else:
                    overflow = True
    return {'passedOriginal': sorted(passed), 'missingOriginal': sorted(ORIGINAL_TESTS - passed),
            'originalPassCount': len(passed), 'packageCoverageComplete': terminal_packages == set(PACKAGES),
            'tests': tests, 'packages': packages, 'observations': observations,
            'invalidJSONRows': invalid, 'invalidProbeLines': invalid_probe, 'failedActions': failures, 'skippedActions': skips,
            'projectionLimitExceeded': overflow}


def run_browser(args, cwd, env, timeout):
    """run_browser runs supplied arguments in an owned process group and returns captured bytes plus bounded cleanup outcomes."""
    started = time.monotonic()
    process = subprocess.Popen(args, cwd=cwd, env=env, stdout=subprocess.PIPE,
                               stderr=subprocess.PIPE, start_new_session=True)
    timed_out, cleanup_unknown = False, False
    try:
        stdout, stderr = process.communicate(timeout=timeout)
    except subprocess.TimeoutExpired:
        timed_out = True
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        except OSError:
            cleanup_unknown = True
        try:
            stdout, stderr = process.communicate(timeout=5)
        except subprocess.TimeoutExpired:
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            except OSError:
                cleanup_unknown = True
            try:
                stdout, stderr = process.communicate(timeout=5)
            except subprocess.TimeoutExpired as error:
                cleanup_unknown = True
                stdout, stderr = error.output or b'', error.stderr or b''
                for pipe in (process.stdout, process.stderr):
                    with contextlib.suppress(Exception):
                        pipe.close()
                with contextlib.suppress(subprocess.TimeoutExpired):
                    process.wait(timeout=5)
    cleanup_unknown = cleanup_unknown or process.returncode is None
    return {'exit': process.returncode, 'watchdogTimeout': timed_out, 'stdout': stdout, 'stderr': stderr,
            'cleanupComplete': not cleanup_unknown, 'cleanupUnknown': cleanup_unknown,
            'streamCaptureComplete': not cleanup_unknown,
            'wallDurationMS': int((time.monotonic() - started) * 1000)}


def evaluate_cell(cell, result):
    """evaluate_cell classifies one cell and captured result without exporting raw stdout or stderr."""
    name, source, mode, serial, instrumented = cell
    stdout, stderr = result.get('stdout', b''), result.get('stderr', b'')
    facts = project(stdout)
    modes = {event['test'] for event in facts['observations']
             if event['kind'] == 'browser_mode' and event['browserMode'] == mode}
    print_success = any(event['test'] == 'TestBrowserPDFWaitsForFontsAndKeepsPagination' and
                        event['kind'] == 'phase' and event['phase'] == 'print' and
                        event['outcome'] == 'success' for event in facts['observations'])
    prerequisite_missing = any(marker in stdout + stderr for marker in (
        b'module lookup disabled by GOPROXY=off', b'GOTOOLCHAIN=local', b'cannot find module providing package'))
    if result.get('notAttempted'):
        status = 'not_attempted_after_unknown_cleanup'
    elif result.get('cleanupUnknown'):
        status = 'watchdog_cleanup_unknown'
    elif result.get('watchdogTimeout'):
        status = 'watchdog_timeout'
    elif result.get('runnerError'):
        status = 'runner_error'
    elif prerequisite_missing:
        status = 'offline_dependency_prerequisite_missing'
    elif result.get('exit') != 0 or facts['failedActions']:
        status = 'test_failure'
    elif facts['skippedActions']:
        status = 'test_skip_not_accepted'
    elif facts['invalidJSONRows'] or facts['invalidProbeLines'] or facts['projectionLimitExceeded']:
        status = 'invalid_or_incomplete_output'
    elif facts['missingOriginal'] or not facts['packageCoverageComplete']:
        status = 'original_coverage_incomplete'
    elif instrumented and (modes != set(CV_TESTS) or not print_success):
        status = 'instrumentation_coverage_incomplete'
    else:
        status = 'passed'
    return dict(facts, cell=name, sourceRole=source, browserMode=mode,
                packageScheduling='p1' if serial else 'default', instrumented=instrumented,
                status=status, exit=result.get('exit'), watchdogTimeout=bool(result.get('watchdogTimeout')),
                cleanupComplete=bool(result.get('cleanupComplete', not result.get('cleanupUnknown', False))),
                cleanupUnknown=bool(result.get('cleanupUnknown')), notAttempted=bool(result.get('notAttempted')),
                streamCaptureComplete=bool(result.get('streamCaptureComplete', not result.get('cleanupUnknown', False))),
                wallDurationMS=result.get('wallDurationMS', 0), stdoutSHA256=digest(stdout),
                stderrSHA256=digest(stderr), stdoutBytes=len(stdout), stderrBytes=len(stderr))


def runner_metadata(environ):
    """runner_metadata returns only validated runner platform/version values from the supplied environment."""
    values = {}
    for key, pattern in (
        ('ImageOS', r'ubuntu(?:22|24|26)(?:[.]04)?'),
        ('ImageVersion', r'[0-9]{8}[.][0-9]{1,5}(?:[.][0-9]{1,5})?'),
    ):
        value = environ.get(key, '')
        values[key] = value if len(value) <= 32 and re.fullmatch(pattern, value) else (
            'unavailable' if not value else 'invalid_omitted')
    return values


def run_matrix(checkouts, environ, execute=run_browser, on_cell=None):
    """run_matrix executes six fixed cells through the supplied runner and returns their aggregate diagnostic report."""
    report = {'schemaVersion': 1, 'startedAtUtc': stamp(), 'complete': False, 'cells': [],
              'watchdogSeconds': WATCHDOG_SECONDS, 'originalTestIdentityCount': len(ORIGINAL_TESTS),
              'dependencyPolicy': 'existing_cache_only_offline_same_inherited_toolchain',
              'paidProductionCalls': False, 'rawOutputPersisted': False,
              'testDeadlinesOrContextsChanged': False, 'testParallelismOverridden': False,
              'runnerMetadata': runner_metadata(environ),
              'prSourceBinding': checkouts.get('prSourceBinding', {'status': 'not_requested'})}
    blocked = False
    for cell in CELLS:
        name, source, mode, serial, instrumented = cell
        environment = environ.copy()
        environment.update(GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local')
        environment.pop(BROWSER_ENV, None)
        if instrumented:
            environment[BROWSER_ENV] = mode
        if blocked:
            result = {'exit': None, 'notAttempted': True, 'stdout': b'', 'stderr': b'',
                      'cleanupUnknown': True, 'cleanupComplete': False, 'streamCaptureComplete': False}
        else:
            try:
                result = execute(command_for(serial), cwd=checkouts[source]['path'],
                                 env=environment, timeout=WATCHDOG_SECONDS)
            except Exception:
                # A runner exception cannot prove no child was started or cleaned up.
                result = {'exit': None, 'runnerError': True, 'stdout': b'', 'stderr': b'',
                          'watchdogTimeout': False, 'cleanupUnknown': True, 'cleanupComplete': False}
        blocked = blocked or bool(result.get('cleanupUnknown'))
        row = evaluate_cell(cell, result)
        row.update(sourceHead=checkouts[source]['head'], sourceTree=checkouts[source]['tree'])
        report['cells'].append(row)
        if on_cell:
            on_cell(report)
    report.update(complete=not blocked, allCellsPassed=all(row['status'] == 'passed' for row in report['cells']),
                  finishedAtUtc=stamp())
    return report


def git(root, *args):
    """git returns captured Git output for supplied arguments or raises a fixed prerequisite marker."""
    process = subprocess.run(['git', '-C', str(root), *args], capture_output=True, timeout=30)
    require(process.returncode == 0, 'git_prerequisite_failed')
    return process.stdout.decode().strip()


@contextlib.contextmanager
def checkouts_for_ci(root, expected_head=None):
    """checkouts_for_ci yields exact owned baseline/current bindings after validating source and module identities."""
    require(not git(root, 'status', '--porcelain'), 'current_checkout_not_clean')
    for revision in (ACCEPTED, EXACT, 'HEAD'):
        for filename, expected in MODULE_BLOBS.items():
            require(git(root, 'rev-parse', revision + ':' + filename) == expected, 'module_bytes_differ')
    created = []
    with tempfile.TemporaryDirectory(prefix='task02-cv-probe-') as temporary:
        owner = pathlib.Path(temporary).resolve()
        paths = {'current': {'path': root, 'head': git(root, 'rev-parse', 'HEAD'),
                             'tree': git(root, 'rev-parse', 'HEAD^{tree}')}}
        if expected_head:
            require(re.fullmatch(r'[0-9a-f]{40}', expected_head), 'expected_pr_head_invalid')
            require(git(root, 'rev-parse', expected_head + '^{commit}') == expected_head,
                    'expected_pr_head_not_available')
            require(git(root, 'rev-parse', expected_head + '^{tree}') == paths['current']['tree'],
                    'checkout_tree_differs_from_expected_pr_head')
            paths['prSourceBinding'] = {'status': 'whole_tree_verified', 'head': expected_head,
                                        'tree': paths['current']['tree']}
        try:
            for role, revision in (('accepted', ACCEPTED), ('exact', EXACT)):
                target = owner / role
                require(owner in target.resolve().parents, 'owned_worktree_path_invalid')
                git(root, 'cat-file', '-e', revision + '^{commit}')
                git(root, 'worktree', 'add', '--detach', str(target), revision)
                created.append(target)
                require(git(target, 'rev-parse', 'HEAD') == revision and
                        not git(target, 'status', '--porcelain'), 'baseline_checkout_not_exact_clean')
                paths[role] = {'path': target, 'head': revision, 'tree': git(target, 'rev-parse', 'HEAD^{tree}')}
            yield paths
            for role in ('accepted', 'exact', 'current'):
                entry = paths[role]
                require(not git(entry['path'], 'status', '--porcelain'), 'source_changed_during_probe')
        finally:
            for target in reversed(created):
                require(owner in target.resolve().parents, 'owned_cleanup_path_invalid')
                git(root, 'worktree', 'remove', '--force', str(target))


def save_report(report, target=REPORT_PATH):
    """save_report serializes the supplied sanitized report to the designated artifact path."""
    target.write_text(json.dumps(report, indent=2) + '\n')


def main():
    """main prepares owned checkouts, records the six-cell probe report and returns a failing status unless every cell passes."""
    root = pathlib.Path(__file__).resolve().parents[1]
    report = {'schemaVersion': 1, 'startedAtUtc': stamp(), 'complete': False, 'cells': [],
              'status': 'setup_started', 'rawOutputPersisted': False}
    save_report(report)
    try:
        with checkouts_for_ci(root, os.environ.get('CV_PRINT_PROBE_EXPECTED_HEAD')) as checkouts:
            report = run_matrix(checkouts, os.environ, on_cell=save_report)
        report['status'] = 'passed' if report['allCellsPassed'] else 'one_or_more_cells_failed'
    except Exception as error:
        report.update(complete=False, allCellsPassed=False, status='setup_or_cleanup_failure',
                      marker=str(error) if isinstance(error, ProbeError) else 'internal_failure')
    save_report(report)
    for row in report.get('cells', []):
        # Every string here is a fixed cell/test identity or a validated finite observation.
        print('CV_PRINT_PROBE_CELL ' + json.dumps(row, sort_keys=True))
    print('CV_PRINT_PROBE_RESULT ' + json.dumps({'complete': report.get('complete', False),
          'allCellsPassed': report.get('allCellsPassed', False), 'status': report['status']}))
    return 0 if report.get('complete') and report.get('allCellsPassed') else 1


if __name__ == '__main__':
    sys.exit(main())
