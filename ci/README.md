# Release deployment contracts

Run the release contracts with Python 3 and PyYAML installed. On Debian or
Ubuntu the dependency is `python3-yaml`; CI installs it explicitly and uses the
distribution interpreter:

```sh
/usr/bin/python3 -B ci/test_release_deployment.py
```

The tests execute the real deployment shell against disposable command doubles.
They check selectors, scoped recreation, inherited environment, image and health
verification, and failure propagation without contacting live services. Parsed
workflow tests cover public browser input/output wiring, publication/cache roles,
target platforms, release triggers, build dependencies, remote target references
and serialization. Comments, YAML quote style and display names are not controls.

Mutation controls retain harmless edits and actual publication, target, security
and failure-path regressions. These local tests do not model GitHub queue scheduling
or replace final-head hosted CI and separately authorized release acceptance.

## Browser dependency controls

`probe_cv_print.py` preserves the inherited accepted and exact source/module
profiles and runs all six original browser cells. The reviewed
`cv-browser-dependency-transition.json` permits only SDK v6 `261c427b230a` to
`6eec9bb2d888`, with exact module and `go.mod` checksums; every other selected
module identity must remain identical. The manifest also binds the concrete
browser and assertion/setup import roots. Changing another dependency or a root
requires a new review and actual browser qualification, not a checksum refresh.

Before browsers start, each profile resolves those roots offline/read-only,
requires selected module directories, module files, archive and zip-hash cache
entries, checks their identities, then runs cache-only `go mod verify`. The latter
checks cached build-list bytes beyond the selected roots; it does not prove absent
modules exist. Missing selected artifacts, unavailable prerequisites, replacements
or mismatched/tampered cache fail closed. Both inherited and candidate SDK cache
profiles must already be present; this driver never downloads dependencies or
changes module files. A clean fresh cache containing only candidate dependencies
may therefore need separately prepared inherited profiles before this offline
acceptance can run. No missing prerequisite is counted as browser success.

Pure Python contracts are `python3 -B ci/test_probe_cv_print.py`; they mock Go/cache
verification and do not qualify real browsers. Actual qualification remains
`python3 -B ci/probe_cv_print.py` with the reviewed source tree, existing cache,
browser tools and coordinated validation lease. Reports keep old and new SDK
profiles distinct; passing inherited controls alone does not accept the update.
