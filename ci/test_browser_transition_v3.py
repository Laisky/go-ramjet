"""Contracts for the exact, source-reviewed browser module transitions."""

import copy
import importlib.util
import json
import pathlib
import tempfile
import unittest
import subprocess
from unittest import mock

SPEC = importlib.util.spec_from_file_location('probe_cv_print', pathlib.Path(__file__).with_name('probe_cv_print.py'))
probe = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(probe)


class BrowserTransitionV3Contracts(unittest.TestCase):
    """Preserve the inherited roots and reject any unreviewed transition."""

    def snapshots(self, policy):
        """Return three synthetic graphs containing every exact reviewed profile."""
        stable = ('example.test/stable', 'v1.0.0', 'h1:stable', 'h1:stablemod')
        return {role: tuple(sorted([(item['module'], *item[role]) for item in probe.reviewed_profiles(policy)] + [stable]))
                for role in ('accepted', 'exact', 'current')}

    def test_manifest_binds_eleven_exact_transitions_and_six_cells(self):
        """Accept only the explicit module identities while retaining all browser cells."""
        policy = probe.reviewed_transition()
        self.assertEqual(policy['schemaVersion'], 3)
        self.assertEqual(len(probe.reviewed_profiles(policy)), 11)
        self.assertEqual(len(probe.CELLS), 6)
        self.assertEqual(len(probe.ORIGINAL_TESTS), 35)
        probe.require_dependency_match(self.snapshots(policy), policy)

    def test_each_unreviewed_profile_change_is_rejected(self):
        """Reject changed versions, checksums, missing rows and unrelated module drift."""
        policy = probe.reviewed_transition()
        for item in probe.reviewed_profiles(policy):
            for role in ('accepted', 'exact', 'current'):
                for column in (1, 2, 3, None):
                    with self.subTest(module=item['module'], role=role, column=column):
                        snapshots = self.snapshots(policy)
                        rows = list(snapshots[role])
                        index = next(i for i, row in enumerate(rows) if row[0] == item['module'])
                        if column is None:
                            rows.pop(index)
                        else:
                            row = list(rows[index])
                            row[column] = 'unreviewed'
                            rows[index] = tuple(row)
                        snapshots[role] = tuple(rows)
                        with self.assertRaises(probe.ProbeError):
                            probe.require_dependency_match(snapshots, policy)
        snapshots = self.snapshots(policy)
        snapshots['current'] = tuple(row if row[0] != 'example.test/stable' else
                                     (row[0], 'v9.9.9', row[2], row[3]) for row in snapshots['current'])
        with self.assertRaisesRegex(probe.ProbeError, 'browser_dependency_graph_differs'):
            probe.require_dependency_match(snapshots, policy)

    def test_manifest_rejects_unknown_duplicate_missing_and_changed_roots(self):
        """Reject policy shape, module-list and control-root changes before preparation."""
        policy = probe.reviewed_transition()
        with tempfile.TemporaryDirectory(prefix='browser-v3-contract-') as temporary:
            path = pathlib.Path(temporary) / 'policy.json'
            mutations = []
            unknown = copy.deepcopy(policy)
            unknown['additionalTransitions'][0]['module'] = 'example.test/unreviewed'
            mutations.append(unknown)
            duplicate = copy.deepcopy(policy)
            duplicate['additionalTransitions'][1] = copy.deepcopy(duplicate['additionalTransitions'][0])
            mutations.append(duplicate)
            missing = copy.deepcopy(policy)
            missing['additionalTransitions'].pop()
            mutations.append(missing)
            root = copy.deepcopy(policy)
            root['controlImports'].append('example.test/unreviewed')
            mutations.append(root)
            checksum = copy.deepcopy(policy)
            checksum['additionalTransitions'][0]['current'][1] = 'bad-checksum'
            mutations.append(checksum)
            for candidate in mutations:
                path.write_text(json.dumps(candidate))
                with self.assertRaises(probe.ProbeError):
                    probe.reviewed_transition(path)

    def test_replaced_or_missing_manifest_fails_before_download(self):
        """Never download when a reviewed profile is replaced or lacks its checksums."""
        policy = probe.reviewed_transition()
        with tempfile.TemporaryDirectory(prefix='browser-v3-prep-') as temporary:
            root = pathlib.Path(temporary)
            (root / 'go.mod').write_text('replace example.test/x => ./local\n')
            (root / 'go.sum').write_text('')
            with mock.patch.object(probe.subprocess, 'run') as execute:
                with self.assertRaises(probe.ProbeError):
                    probe.prepare_dependency_cache(root, 'current', policy)
                execute.assert_not_called()

    def test_preparation_downloads_only_the_exact_reviewed_profiles(self):
        """Bind all role-specific module and checksum tuples before any download."""
        policy = probe.reviewed_transition()
        profiles = probe.reviewed_profiles(policy)
        for role in ('accepted', 'exact', 'current'):
            with self.subTest(role=role), tempfile.TemporaryDirectory(prefix='browser-v3-prep-') as temporary:
                root = pathlib.Path(temporary)
                (root / 'go.mod').write_text('\n'.join(item['module'] + ' ' + item[role][0] for item in profiles))
                (root / 'go.sum').write_text('\n'.join(item['module'] + ' ' + item[role][0] + suffix + ' ' + checksum
                    for item in profiles for suffix, checksum in (('', item[role][1]), ('/go.mod', item[role][2]))))
                wanted = {item['module'] + '@' + item[role][0]: item for item in profiles}
                def download(args, **kwargs):
                    """Return the matching pinned metadata without contacting a registry."""
                    item = wanted[args[-1]]
                    metadata = dict(zip(('Path', 'Version', 'Sum', 'GoModSum'), (item['module'], *item[role])))
                    return subprocess.CompletedProcess(args, 0, json.dumps(metadata).encode(), b'')
                with mock.patch.object(probe.subprocess, 'run', side_effect=download) as execute:
                    result = probe.prepare_dependency_cache(root, role, policy)
                self.assertEqual(execute.call_count, 11)
                self.assertEqual({call.args[0][-1] for call in execute.call_args_list}, set(wanted))
                self.assertEqual(result['status'], 'cache_prepared_not_qualified')

    def test_each_missing_or_altered_profile_fails_before_download(self):
        """An altered version or checksum for any reviewed module must fail closed."""
        policy = probe.reviewed_transition()
        profiles = probe.reviewed_profiles(policy)
        for item in profiles:
            for mutation in ('version', 'checksum', 'missing'):
                with self.subTest(module=item['module'], mutation=mutation), tempfile.TemporaryDirectory(prefix='browser-v3-prep-') as temporary:
                    root = pathlib.Path(temporary)
                    mod = '\n'.join(p['module'] + ' ' + p['current'][0] for p in profiles)
                    sums = '\n'.join(p['module'] + ' ' + p['current'][0] + suffix + ' ' + checksum
                        for p in profiles for suffix, checksum in (('', p['current'][1]), ('/go.mod', p['current'][2])))
                    if mutation == 'version':
                        mod = mod.replace(item['module'] + ' ' + item['current'][0], item['module'] + ' v9.9.9')
                    elif mutation == 'checksum':
                        sums = sums.replace(item['current'][1], 'h1:unreviewed')
                    else:
                        sums = '\n'.join(line for line in sums.splitlines() if not line.startswith(item['module'] + ' '))
                    (root / 'go.mod').write_text(mod)
                    (root / 'go.sum').write_text(sums)
                    with mock.patch.object(probe.subprocess, 'run') as execute:
                        with self.assertRaises(probe.ProbeError):
                            probe.prepare_dependency_cache(root, 'current', policy)
                        execute.assert_not_called()


if __name__ == '__main__':
    unittest.main()
