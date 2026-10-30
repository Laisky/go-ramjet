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
`cv-browser-dependency-transition.json` permits exactly SDK v6 `261c427b230a` to
`6eec9bb2d888` and the shared crawler module `laisky-blog-graphql` from
`d58d6c75a705` to `f1aa1b9483de`, each with exact module and `go.mod` checksums.
Every other selected module identity must remain identical. Schema version 2
adds only this named companion; the SDK-only version 1 format remains supported.
The manifest also binds the concrete
browser and assertion/setup import roots. Changing another dependency or a root
requires a new review and actual browser qualification, not a checksum refresh.

Before browsers start, each profile resolves those roots offline/read-only,
requires selected module directories, module files, archive and zip-hash cache
entries, checks their identities, then runs cache-only `go mod verify`. The latter
checks cached build-list bytes beyond the selected roots; it does not prove absent
modules exist. Missing selected artifacts, unavailable prerequisites, replacements
or mismatched/tampered cache fail closed. Both inherited and candidate SDK and
shared crawler cache profiles must be present before the default offline verifier runs. The explicit
`python3 -B ci/probe_cv_print.py --prepare-cache` preparation step binds the same
source identities and exact reviewed profiles, copies their immutable module
files into owned temporary directories, and runs public checksum-verified
`go mod download -json <exact-reviewed-module>@<profile-version>` for each of
the two transitioning modules, with a 180-second bound per download. Its returned path/version/module checksum and
module-file checksum must exactly match the reviewed tuple. Ordinary current
CI dependency setup supplies the other 65 selected modules; preparation does
not expand to historical unused dependency versions or rewrite their checksums. It fixes
GOPROXY to `https://proxy.golang.org` without direct fallback, GOSUMDB to
`sum.golang.org`, GOAUTH off, no private/proxy exclusions, local toolchain and
readonly/no-workspace flags. Original and copied manifests must remain byte
identical. Replacements, profile mismatches, timeouts and download errors fail
visibly without running browsers or claiming qualification. The PR workflow
prepares profiles explicitly, then invokes the unchanged offline verifier.
Preparation reports are separate and say `cache_prepared_not_qualified`. A cache
containing only candidate dependencies lacks the old SDK and shared crawler needed by inherited
controls; it is not evidence of a browser regression. No missing prerequisite is counted as browser success.

Pure Python contracts are `python3 -B ci/test_probe_cv_print.py`; they mock Go/cache
verification and do not qualify real browsers. Actual qualification remains
`python3 -B ci/probe_cv_print.py` with the reviewed source tree, existing cache,
browser tools and coordinated validation lease. Reports keep the original SDK
fields and add distinct shared crawler profiles; passing inherited controls alone
does not accept the update. The SDK-only schema keeps its original report shape.

## Exact browser graph transition for the dependency update

Schema 3 retains the historical SDK and crawler profiles, roots, and six cells.
It also binds exactly nine additional selected modules, each with accepted, exact
and current version/module/go.mod checksums. Gin is a direct browser control root;
MIME detection is selected through Gin validation; deque is selected by the SDK;
x/crypto, x/net, x/sync, x/sys, x/term and x/text are selected transitive packages.
All other module identities remain identical. Unknown modules, changed roots,
replacements, missing artifacts and mismatched checksums still fail closed.
The six-cell browser run is required; cache preparation alone is not acceptance.
