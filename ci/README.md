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
