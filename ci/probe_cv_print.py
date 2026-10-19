#!/usr/bin/env python3
"""Run six distinct synthetic browser controls; never replay a production fetch.

This module has no import side effects. Only the hosted CI entry point invokes
browsers. Raw Go test output stays in memory for hashes and finite projections.
"""

import base64
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
CONTROL_IMPORTS = ('github.com/chromedp/chromedp', 'github.com/chromedp/cdproto/page',
                   'github.com/chromedp/cdproto/runtime', 'github.com/chromedp/cdproto/fetch',
                   'github.com/chromedp/cdproto/network',
                   'github.com/Laisky/laisky-blog-graphql/library/crawleregress',
                   'github.com/yuin/goldmark/v2/renderer/html',
                   # Concrete imports used by the preserved PDF assertions and renderer test setup.
                   'github.com/pdfcpu/pdfcpu/pkg/api', 'github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model',
                   'github.com/stretchr/testify/require', 'github.com/Laisky/gin-middlewares/v7',
                   'github.com/Laisky/laisky-blog-graphql/library/db/redis')
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
    """A fixed marker and optional already-sanitized dirty metadata."""

    def __init__(self, marker, dirty_metadata=None):
        """__init__ stores a fixed marker and finite metadata without raw command output."""
        super().__init__(marker)
        self.dirty_metadata = dirty_metadata


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
              'prSourceBinding': checkouts.get('prSourceBinding', {'status': 'not_requested'}),
              'dependencyBinding': checkouts.get('dependencyBinding', {'status': 'not_requested'})}
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


SAFE_EXTENSIONS = frozenset(('.go', '.py', '.pyc', '.pyo', '.json', '.yml', '.yaml', '.md', '.txt',
                             '.log', '.xml', '.out', '.test', '.tmp', '.pdf', '.html', '.css', '.sql',
                             '.js', '.jsx', '.ts', '.tsx', '.mjs', '.mts', '.sh', '.toml', '.mod', '.sum'))
PUBLIC_ROOT_FILES = frozenset(('go.mod', 'go.sum', 'Dockerfile', 'Makefile', 'README.md', 'AGENTS.md'))
PUBLIC_PREFIXES = frozenset(('ci', 'internal', 'library', 'cmd', 'docs', 'web', '.scripts'))
SENSITIVE_COMPONENT = re.compile(r'secret|credential|password|token|api[_-]?key|cookie|private|instruction', re.IGNORECASE)


def public_source_path(path, tracked):
    """public_source_path permits only tracked, bounded ASCII source paths without sensitive components."""
    if path not in tracked:
        return False
    try:
        text = path.decode('ascii')
    except UnicodeDecodeError:
        return False
    if len(text) > 256 or not re.fullmatch(r'[A-Za-z0-9_./@+-]+', text):
        return False
    parts = pathlib.PurePosixPath(text).parts
    if not parts or text.startswith('/') or any(part in ('.', '..') or SENSITIVE_COMPONENT.search(part) for part in parts):
        return False
    if text in PUBLIC_ROOT_FILES:
        return True
    if parts[0] == '.github':
        allowed_prefix = len(parts) > 2 and parts[1] == 'workflows'
    else:
        allowed_prefix = parts[0] in PUBLIC_PREFIXES
    hidden = any(part.startswith('.') for index, part in enumerate(parts) if index > 0)
    return bool(allowed_prefix and not hidden and pathlib.PurePosixPath(text).suffix.lower() in SAFE_EXTENSIONS)


def dirty_metadata(status, tracked_output):
    """dirty_metadata projects porcelain-v1 NUL records to counts and at most 32 privacy-safe rows."""
    tracked = set(tracked_output.split(b'\0')) - {b''}
    records = status.split(b'\0')
    rows, status_counts, category_counts = [], {}, {}
    index, total, malformed = 0, 0, 0
    while index < len(records):
        record = records[index]
        index += 1
        if not record:
            continue
        valid = len(record) >= 4 and record[2:3] == b' ' and re.fullmatch(rb'[ MADRCUT?!]{2}', record[:2])
        code = record[:2].decode('ascii') if valid else 'unknown'
        path = record[3:] if valid else record
        if not valid:
            malformed += 1
        if valid and ('R' in code or 'C' in code):
            if index < len(records) and records[index]:
                index += 1  # Old rename/copy path is deliberately not exported.
            else:
                malformed += 1
        total += 1
        status_counts[code] = status_counts.get(code, 0) + 1
        text = path.decode('utf-8', errors='replace')
        parts = pathlib.PurePosixPath(text).parts
        prefix = parts[0] if parts and parts[0] in PUBLIC_PREFIXES else (
            'github_workflow' if len(parts) > 1 and parts[:2] == ('.github', 'workflows') else 'other')
        extension = pathlib.PurePosixPath(text).suffix.lower()
        extension = extension if extension in SAFE_EXTENSIONS else ('none' if not extension else 'other')
        is_tracked = path in tracked
        is_public = public_source_path(path, tracked)
        category = 'tracked_public_source' if is_public else 'tracked_other' if is_tracked else (
            'python_cache' if '__pycache__' in parts or extension in ('.pyc', '.pyo') else 'untracked_other')
        row = {'status': code, 'tracked': is_tracked, 'category': category,
               'prefixCategory': prefix, 'extension': extension, 'pathSHA256': digest(path)}
        if is_public:
            row['path'] = text
        elif category == 'python_cache':
            match = re.fullmatch(r'(.+)/__pycache__/([A-Za-z_][A-Za-z0-9_]*)[.]cpython-[0-9]{2,3}(?:[.]opt-[0-9])?[.]pyc', text)
            if match:
                source = (match[1] + '/' + match[2] + '.py').encode()
                if public_source_path(source, tracked):
                    row['trackedSourcePath'] = source.decode('ascii')
        category_counts[category] = category_counts.get(category, 0) + 1
        if len(rows) < 32:
            rows.append(row)
    return {'totalEntries': total, 'statusCounts': status_counts, 'categoryCounts': category_counts,
            'entries': rows, 'entryLimit': 32, 'entryLimitReached': total > 32,
            'malformedRecords': malformed, 'contentsRead': False}


def git_bytes(root, *args):
    """git_bytes returns unprinted Git metadata bytes or raises a fixed prerequisite marker."""
    process = subprocess.run(['git', '-C', str(root), *args], capture_output=True, timeout=30)
    require(process.returncode == 0, 'git_prerequisite_failed')
    return process.stdout


def git(root, *args):
    """git returns captured Git output for supplied arguments or raises a fixed prerequisite marker."""
    return git_bytes(root, *args).decode().strip()


def require_clean(root, marker):
    """require_clean preserves the fatal dirty guard and attaches only bounded file-status metadata."""
    status = git_bytes(root, 'status', '--porcelain=v1', '-z', '--untracked-files=all')
    if status:
        tracked = git_bytes(root, 'ls-files', '--cached', '-z')
        raise ProbeError(marker, dirty_metadata(status, tracked))



def dependency_graph(output, checksums):
    """dependency_graph returns selected browser module identities and their tracked checksums only."""
    sums = {}
    for line in checksums.splitlines():
        parts = line.split()
        require(len(parts) == 3, 'dependency_checksum_invalid')
        key = tuple(parts[:2])
        require(key not in sums or sums[key] == parts[2], 'dependency_checksum_conflict')
        sums[key] = parts[2]
    selected = {}
    for line in output.splitlines():
        parts = line.split()
        if not parts:
            continue
        require(len(parts) == 2, 'browser_dependency_replaced_or_invalid')
        path, version = parts
        require(path not in selected or selected[path] == version, 'browser_dependency_graph_invalid')
        selected[path] = version
    require(bool(selected), 'browser_dependency_graph_empty')
    graph = []
    for path, version in sorted(selected.items()):
        require((path, version) in sums and (path, version + '/go.mod') in sums,
                'browser_dependency_checksum_missing')
        graph.append((path, version, sums[(path, version)], sums[(path, version + '/go.mod')]))
    return tuple(graph)


def dependency_snapshot(root):
    """dependency_snapshot resolves browser dependencies offline without changing module metadata."""
    environment = os.environ.copy()
    environment.update(GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GONOPROXY='none')
    template = '{{if .Module}}{{.Module.Path}} {{.Module.Version}}{{if .Module.Replace}} replaced{{end}}{{end}}'
    try:
        process = subprocess.run(['go', 'list', '-mod=readonly', '-deps', '-f', template, *CONTROL_IMPORTS],
                                 cwd=root, env=environment, capture_output=True, timeout=60)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ProbeError('browser_dependency_graph_unavailable') from error
    require(process.returncode == 0, 'browser_dependency_graph_unavailable')
    try:
        return dependency_graph(process.stdout.decode(), (root / 'go.sum').read_text())
    except UnicodeError as error:
        raise ProbeError('browser_dependency_graph_invalid') from error


def reviewed_transition(path=None):
    """reviewed_transition loads the source-reviewed roots and exact SDK profiles, never runtime policy."""
    target = path or pathlib.Path(__file__).with_name('cv-browser-dependency-transition.json')
    try:
        policy = json.loads(target.read_text())
    except (OSError, ValueError) as error:
        raise ProbeError('browser_dependency_policy_unavailable') from error
    require(isinstance(policy, dict), 'browser_dependency_policy_invalid')
    require(set(policy) == {'schemaVersion', 'controlImports', 'module', 'accepted', 'exact', 'current'},
            'browser_dependency_policy_invalid')
    require(policy['schemaVersion'] == 1 and policy['module'] == 'github.com/Laisky/go-utils/v6',
            'browser_dependency_policy_invalid')
    require(policy['controlImports'] == list(CONTROL_IMPORTS), 'browser_dependency_roots_differ')
    for role in ('accepted', 'exact', 'current'):
        row = policy[role]
        require(isinstance(row, list) and len(row) == 3 and all(isinstance(value, str) for value in row),
                'browser_dependency_policy_invalid')
        require(bool(re.fullmatch(r'v6[.][0-9]+[.][0-9]+-0[.][0-9]{14}-[0-9a-f]{12}', row[0])) and
                all(re.fullmatch(r'h1:[A-Za-z0-9+/]{43}=', value) for value in row[1:]),
                'browser_dependency_policy_invalid')
    require(policy['accepted'] == policy['exact'] and policy['current'] != policy['accepted'],
            'browser_dependency_policy_invalid')
    return policy


def require_dependency_match(snapshots, policy=None):
    """require_dependency_match permits only the exact reviewed SDK transition; every other tuple stays identical."""
    require(set(snapshots) == {'accepted', 'exact', 'current'}, 'browser_dependency_roles_incomplete')
    if policy is None:
        require(snapshots['accepted'] == snapshots['exact'] == snapshots['current'],
                'browser_dependency_graph_differs')
        return
    module = policy['module']
    for role in ('accepted', 'exact', 'current'):
        actual = tuple(row for row in snapshots[role] if row[0] == module)
        require(actual == ((module, *policy[role]),), 'browser_sdk_profile_differs')
    require(snapshots['accepted'] == snapshots['exact'], 'browser_dependency_graph_differs')
    normalized = tuple((module, *policy['accepted']) if row[0] == module else row for row in snapshots['current'])
    require(snapshots['accepted'] == normalized, 'browser_dependency_graph_differs')


def verify_dependency_cache(root, graph):
    """verify_dependency_cache requires selected cached artifacts and verifies cached bytes without fetching."""
    environment = os.environ.copy()
    environment.update(GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local', GONOPROXY='none', GOFLAGS='-mod=readonly')
    modules = [row[0] for row in graph]
    try:
        metadata = subprocess.run(['go', 'list', '-mod=readonly', '-m', '-json', *modules],
                                  cwd=root, env=environment, capture_output=True, timeout=60)
        require(metadata.returncode == 0, 'browser_dependency_cache_unavailable')
        decoder, pending, found = json.JSONDecoder(), metadata.stdout.decode(), {}
        while pending.strip():
            entry, consumed = decoder.raw_decode(pending.lstrip())
            pending = pending.lstrip()[consumed:]
            require(isinstance(entry, dict) and entry.get('Path') not in found,
                    'browser_dependency_cache_invalid')
            found[entry.get('Path')] = entry
        require(set(found) == set(modules), 'browser_dependency_cache_incomplete')
        for module, version, checksum, mod_checksum in graph:
            entry = found[module]
            require(not entry.get('Replace') and not entry.get('Error'), 'browser_dependency_cache_invalid')
            require((entry.get('Version'), entry.get('Sum'), entry.get('GoModSum')) ==
                    (version, checksum, mod_checksum), 'browser_dependency_cache_identity_differs')
            directory, modfile = pathlib.Path(entry.get('Dir', '')), pathlib.Path(entry.get('GoMod', ''))
            require(bool(entry.get('Dir')) and bool(entry.get('GoMod')) and directory.is_dir() and modfile.is_file(),
                    'browser_dependency_cache_incomplete')
            archive, ziphash = modfile.with_suffix('.zip'), modfile.with_suffix('.ziphash')
            require(archive.is_file() and ziphash.is_file(), 'browser_dependency_cache_incomplete')
            require(ziphash.read_text().strip() == checksum, 'browser_dependency_cache_identity_differs')
            # Go's module-file checksum is dirhash.Hash1 over the single name go.mod.
            line = hashlib.sha256(modfile.read_bytes()).hexdigest() + '  go.mod\n'
            actual = 'h1:' + base64.b64encode(hashlib.sha256(line.encode()).digest()).decode()
            require(actual == mod_checksum, 'browser_dependency_cache_identity_differs')
        verified = subprocess.run(['go', 'mod', 'verify'], cwd=root, env=environment,
                                  capture_output=True, timeout=120)
        require(verified.returncode == 0, 'browser_dependency_cache_verify_failed')
    except ProbeError:
        raise
    except (OSError, ValueError, UnicodeError, subprocess.TimeoutExpired) as error:
        raise ProbeError('browser_dependency_cache_unavailable') from error


@contextlib.contextmanager
def checkouts_for_ci(root, expected_head=None):
    """checkouts_for_ci yields exact owned baseline/current bindings after validating source and module identities."""
    require_clean(root, 'current_checkout_not_clean')
    policy = reviewed_transition()
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
                require(git(target, 'rev-parse', 'HEAD') == revision, 'baseline_checkout_not_exact_clean')
                require_clean(target, 'baseline_checkout_not_exact_clean')
                paths[role] = {'path': target, 'head': revision, 'tree': git(target, 'rev-parse', 'HEAD^{tree}')}
            snapshots = {}
            for role in ('accepted', 'exact', 'current'):
                snapshots[role] = dependency_snapshot(paths[role]['path'])
                verify_dependency_cache(paths[role]['path'], snapshots[role])
                require_clean(paths[role]['path'], 'module_metadata_changed_during_graph')
            require_dependency_match(snapshots, policy)
            paths['dependencyBinding'] = {'status': 'reviewed_sdk_transition_verified',
                                          'moduleCount': len(snapshots['current']),
                                          'sdkModule': policy['module'], 'acceptedSDK': policy['accepted'],
                                          'exactSDK': policy['exact'], 'currentSDK': policy['current'],
                                          'cacheVerification': 'selected_artifacts_present_and_go_mod_verify'}
            yield paths
            for role in ('accepted', 'exact', 'current'):
                entry = paths[role]
                require_clean(entry['path'], 'source_changed_during_probe')
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
        if isinstance(error, ProbeError) and error.dirty_metadata is not None:
            report['dirtyCheckout'] = error.dirty_metadata
    save_report(report)
    for row in report.get('cells', []):
        # Every string here is a fixed cell/test identity or a validated finite observation.
        print('CV_PRINT_PROBE_CELL ' + json.dumps(row, sort_keys=True))
    print('CV_PRINT_PROBE_RESULT ' + json.dumps({'complete': report.get('complete', False),
          'allCellsPassed': report.get('allCellsPassed', False), 'status': report['status']}))
    return 0 if report.get('complete') and report.get('allCellsPassed') else 1


if __name__ == '__main__':
    sys.exit(main())
