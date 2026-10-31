"""Fresh-cache regression for browser manifest graph preparation."""

import importlib.util
import os
import pathlib
import subprocess
import tempfile
import unittest
import zipfile
from unittest import mock

spec = importlib.util.spec_from_file_location('probe_cv_print', pathlib.Path(__file__).with_name('probe_cv_print.py'))
probe = importlib.util.module_from_spec(spec)
spec.loader.exec_module(probe)


class FreshCacheGraph(unittest.TestCase):
    def test_full_graph_preparation_and_integrity_control(self):
        with tempfile.TemporaryDirectory(prefix='cv-cache-graph-test-') as tmp:
            owner = pathlib.Path(tmp)
            proxy = owner / 'proxy' / 'example.com' / 'cachefixture' / '@v'
            proxy.mkdir(parents=True)
            module = 'example.com/cachefixture'
            version = 'v1.0.0'
            gomod = 'module ' + module + '\n\ngo 1.20\n'
            (proxy / (version + '.mod')).write_text(gomod)
            (proxy / (version + '.info')).write_text('{"Version":"v1.0.0","Time":"2020-01-01T00:00:00Z"}')
            with zipfile.ZipFile(proxy / (version + '.zip'), 'w') as archive:
                archive.writestr(module + '@' + version + '/go.mod', gomod)
                archive.writestr(module + '@' + version + '/fixture.go', 'package cachefixture\n')
            root = owner / 'root'
            root.mkdir()
            (root / 'go.mod').write_text('module example.com/main\n\ngo 1.20\n\nrequire ' + module + ' ' + version + '\n')
            base = os.environ.copy()
            base.update(GOPROXY=(owner / 'proxy').as_uri(), GOSUMDB='off', GONOPROXY='none',
                        GONOSUMDB='none', GOAUTH='off', GOTOOLCHAIN='local', GOWORK='off', GOFLAGS='-mod=readonly',
                        GOMODCACHE=str(owner / 'seed'))
            subprocess.run(['go', 'mod', 'download'], cwd=root, env=base, check=True, capture_output=True)
            before = [(root / name).read_bytes() for name in ('go.mod', 'go.sum')]
            env = dict(base, GOMODCACHE=str(owner / 'fresh'))
            offline = dict(env, GOPROXY='off')
            red = subprocess.run(['go', 'mod', 'verify'], cwd=root, env=offline, capture_output=True)
            self.assertNotEqual(red.returncode, 0, 'fresh offline cache must not already verify')
            real_run = subprocess.run

            def local_proxy(args, **kwargs):
                altered = dict(kwargs['env'])
                altered.update(GOPROXY=env['GOPROXY'], GOSUMDB='off', GOMODCACHE=env['GOMODCACHE'])
                return real_run(args, **dict(kwargs, env=altered))

            with mock.patch.object(probe.subprocess, 'run', side_effect=local_proxy):
                probe.prepare_manifest_graph(root)
            self.assertEqual([(root / name).read_bytes() for name in ('go.mod', 'go.sum')], before)
            green = subprocess.run(['go', 'mod', 'verify'], cwd=root, env=offline, capture_output=True)
            self.assertEqual(green.returncode, 0, green.stderr)
            extracted = owner / 'fresh' / 'example.com' / 'cachefixture@v1.0.0'
            extracted.chmod(0o755)
            marker = extracted / 'tamper.go'
            marker.write_text('package cachefixture\n')
            tampered = subprocess.run(['go', 'mod', 'verify'], cwd=root, env=offline, capture_output=True)
            self.assertNotEqual(tampered.returncode, 0, 'modified module must fail verification')
            marker.unlink()
            extracted.chmod(0o555)
            restored = subprocess.run(['go', 'mod', 'verify'], cwd=root, env=offline, capture_output=True)
            self.assertEqual(restored.returncode, 0, restored.stderr)


    def test_download_failures_and_manifest_drift_fail_closed(self):
        for outcome in ('failure', 'timeout', 'changed_copy'):
            with self.subTest(outcome=outcome), tempfile.TemporaryDirectory(prefix='cv-graph-failure-') as tmp:
                root = pathlib.Path(tmp)
                (root / 'go.mod').write_text('module example.com/main\n')
                (root / 'go.sum').write_text('original\n')

                def failed_download(args, **kwargs):
                    self.assertEqual(args, ['go', 'mod', 'download'])
                    env = kwargs['env']
                    self.assertEqual(env['GOPROXY'], 'https://proxy.golang.org')
                    self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
                    self.assertEqual(env['GOAUTH'], 'off')
                    self.assertEqual(env['GOFLAGS'], '-mod=readonly')
                    if outcome == 'timeout':
                        raise subprocess.TimeoutExpired(args, 300)
                    if outcome == 'changed_copy':
                        (kwargs['cwd'] / 'go.sum').write_text('changed')
                    return subprocess.CompletedProcess(args, int(outcome == 'failure'), b'', b'private-canary')

                with mock.patch.object(probe.subprocess, 'run', side_effect=failed_download):
                    with self.assertRaises(probe.ProbeError) as caught:
                        probe.prepare_manifest_graph(root)
                self.assertNotIn('private-canary', str(caught.exception))
                self.assertEqual((root / 'go.sum').read_text(), 'original\n')


if __name__ == '__main__':
    unittest.main()
