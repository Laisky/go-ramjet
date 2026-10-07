# October 2026 stable dependency upgrade

PR #74 updates the application using released versions verified on October 7, 2026. Its acceptance evidence is recorded on the PR. A successful preparation
workflow alone is not proof of passing tests: diagnostic runs retain individual
exit codes. The final normal and Bolt verification workflows must both pass.

## Toolchain and native libraries

Go 1.27.1 is used by `go.mod`, the Docker build, and repository verification.
Node.js 24.21.0 is the selected production LTS runtime; the non-LTS Current line
is not adopted simply because its major number is larger. pnpm is pinned at
12.9.1 in both entry-point manifests. Developer analysis tools are pinned in the
Makefile to the same versions as CI.

The Azure Speech Go binding and native archive are both 1.52.0. Updating only
one side can produce C API or runtime linker failures. Docker and both CI
workflows must keep their native SDK version aligned with the Go module.
GoJieba 1.4.7 remains the latest released version, and its runtime dictionary
files remain aligned with the module copied by the Docker build.

## Major API migrations

- chromedp 0.20.1 and cdproto 0.157.9 use typed commands. Void actions use `Do`,
  result actions use `Run`, and protocol commands use `Call` with parameter
  structs. Browser JavaScript exceptions are represented by `ExceptionError`.
  The crawler retains headers, readiness polling, cancellation, and rendered
  HTML extraction. PDF printing still awaits the font promise and preserves
  CSS page sizes, backgrounds, and empty-output checks.
- Goldmark 2.1.6 separates parsing from HTML rendering. The CV renderer retains
  GFM tables, task lists, strikethrough, heading IDs, and its pre-existing raw
  HTML policy. Tests exercise parallel reuse without heading-ID leakage.
- html-to-markdown 2.5.2 uses its v2 conversion entry point. A semantic round-trip
  test covers headings, emphasis, links, lists, and code without external LLMs.
- pdfcpu 0.16.1 receives the caller's context in merge and page-count operations.
  Tests cover valid pagination, invalid inputs, and cancelled merges.
- Stripe Go v87 retains the existing PaymentIntent contract and now receives
  the HTTP request context. A local transport verifies the request form, API
  version header, response decoding, and upstream error handling. Tests never
  contact Stripe or create real payments. Stripe React 7 and Stripe.js 10 are
  validated by the frontend build and application tests.
- Application set imports use golang-set v3.0.0. Dependencies that still import
  its v2 module retain the latest compatible v2 release independently.

## Version boundaries that must not be hidden

MongoDB's v1 driver remains at its latest v1 release, 1.17.10, because the
external `laisky-blog-graphql` integration exposes v1 collection and BSON types
in its public API. The graph also contains v2.9.2 for consumers already migrated
to v2. Replacing v1 with v2 is not an import-only change; it needs a coordinated
migration of that external API. No blanket cross-major `replace` is used.

The upstream gofpdf release `v2.7.1` still declares a v1 module path. Its exact
released commit `95d7704723b18e9b55cd71b234a2e40815e990d6` is therefore recorded
by Go as `v1.4.4-0.20260415234927-95d7704723b1`. This is the stable release's
source, not an arbitrary development snapshot.

Existing untagged project forks retain their required pseudo-versions when no
newer stable tag exists. A lower stable tag must not silently remove fixes from
an existing pinned commit. Legacy module paths needed by dependencies remain
in the graph rather than being forced across incompatible major versions.

The frontend uses TypeScript 7.0.2 for compilation and the supported TypeScript
6.0.2 Compiler API alias for ESLint. See [frontend compatibility](../web/DEPENDENCIES.md)
for package aliases, pnpm workspace settings, lockfile documents, rendering
compatibility, and request-lifecycle tests.

## Reproducing acceptance

Install the native Speech SDK and set its include/linker/runtime search paths
as shown in `.github/workflows/pr-validation.yml`, then run:

```sh
make install
make lint
go test ./... -count=1
corepack enable
pnpm -C web install --frozen-lockfile
pnpm -C web lint
pnpm -C web test
pnpm -C web build
```

Real-browser integration requires Chrome or Chromium, `pdftotext`, and a Linux
D-Bus session. It uses self-contained HTML and a local HTTP fixture:

```sh
dbus-run-session -- go test -tags=browser_integration \
  ./internal/tasks/cv ./internal/tasks/gptchat/tasks \
  -run '^TestBrowser' -count=1
```

These tests verify JavaScript-generated content, request headers, font-promise
completion and rejection, PDF content, and two-page pagination. The pipeline
also retains SPA metadata benchmark thresholds and builds the production
Docker image. Bolt verification independently checks repository cleanliness,
tests, lint, the image, and embedded VCS identity. Image validation does not
by itself confirm deployment.

Vulnerability scanning fails normally on tool errors or called vulnerable code;
the old panic-success workaround is removed. Distinguish called-code findings,
imported-package findings, and uncalled module-level advisories when reporting
scan results. A successful scan does not imply every required module has no
advisories.
