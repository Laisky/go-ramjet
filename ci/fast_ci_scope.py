"""Select browser smoke for changes that can affect browser behavior."""

import os
import pathlib
import subprocess
import sys


def browser_related(path):
    """browser_related returns whether a changed path can affect browser coverage."""
    name = pathlib.PurePosixPath(path)
    return (name.suffix == '.go' or path in ('go.mod', 'go.sum', 'Dockerfile')
            or path.startswith(('internal/tasks/cv/', 'internal/tasks/gptchat/tasks/', 'cmd/crawler-browser-launcher/'))
            or (path.startswith('ci/') and name.suffix in ('.py', '.json'))
            or path in ('ci/fast_ci_scope.py', 'ci/test_fast_ci.py',
                        'ci/cv-browser-dependency-transition.json',
                        '.github/workflows/pr-validation.yml'))


def main():
    """main emits a fail-closed browser-smoke decision for an exact Git diff."""
    base, head = sys.argv[1:]
    changes = subprocess.run(['git', 'diff', '--name-only', '--no-renames', '-z', base, head, '--'],
                             check=True, capture_output=True).stdout.split(b'\0')
    paths = [os.fsdecode(path) for path in changes if path]
    decision = any(browser_related(path) for path in paths)
    with open(os.environ['GITHUB_OUTPUT'], 'a', encoding='utf-8') as output:
        output.write('browser=' + str(decision).lower() + '\n')


if __name__ == '__main__':
    main()
