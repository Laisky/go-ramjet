# Frontend dependency compatibility

## Reproducible commands

Use Node.js 24.21.0, the production and CI LTS runtime, and enable Corepack.
The repository root and `web/package.json` both pin pnpm 12.9.1. The root
manifest exists because Corepack selects its executable before pnpm processes
`-C web`; omitting the root pin can select an incompatible major version.

From the repository root:

```sh
corepack enable
pnpm -C web install --frozen-lockfile
pnpm -C web lint
pnpm -C web test
pnpm -C web build
```

Update both package-manager pins together. Their regression test checks that
they remain identical. pnpm 12 reads the strict peer-dependency policy,
transitive overrides, and dependency build-script policy from
`web/pnpm-workspace.yaml`; do not move these settings back to `.npmrc` or
`package.json`. Dependency build scripts are not enabled by this upgrade.

## Native TypeScript and lint API

The build uses TypeScript 7.0.2's native compiler. ESLint's TypeScript parser
still requires the JavaScript Compiler API, so the project uses Microsoft's
[documented dual-package setup](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/):

- `@typescript/native` aliases `typescript@^7.0.2` and provides the compiler.
- `typescript` aliases `@typescript/typescript6@^6.0.2` for API consumers.

The 6.0.2 compatibility package currently re-exports the locked 6.0.3 API.
Keep both entries. Installing only the native compiler as `typescript` breaks
API-dependent lint tools; bypassing peer checks is not a fix. TypeScript 7 no
longer accepts `baseUrl`, so aliases use explicit relative `paths` entries.
Vite and Vitest configuration uses `import.meta.dirname` for native ESM loading.

## Dependency update policy

PR #70 reconciled dependency PRs #44, #47, #52, #53, #55, #57, #61, #62, and #64
on current application code instead of merging nine stale lockfiles. PR #72
integrated #71 and #73 with math-rendering and request-lifecycle fixes. PR #74
updates the stable toolchain and major dependencies, including React 19.3,
Mermaid 12.1, Vitest 5.0.3, and ESLint 10.12.

Generate the lockfile with pnpm, then format it with the repository's Prettier
configuration. Never edit integrity hashes manually. Conditional transitive
minimum overrides retain compatible version ranges; avoid an exact override
that rejects a newer compatible peer dependency.

pnpm 12 writes multiple YAML documents: toolchain metadata and the application
graph. `readResolvedPackages` parses every document, not just the first
`packages` section. Regression tests cover scoped/quoted names, inline mappings,
peer-qualified keys, malformed metadata, and exclusion of snapshot entries.
Mandatory application packages must be present, and every resolved copy must
meet its minimum. Historical same-major floors remain enforced when present;
a removed transitive package does not need to be reintroduced.

## Mermaid compatibility

Mermaid 11.15 changed dotted class-diagram namespaces into hierarchical groups
by default. Existing diagrams use flat labels such as `api.v1`.
`createMermaidConfig` continues to disable automatic hierarchical namespaces
with Mermaid 12, retaining strict security and both color themes. Tests exercise
the real parser and layout data, flowcharts, sequence diagrams, and invalid input.

During PR #70 preparation, removing that setting failed both namespace-theme
tests; restoring the pre-update lockfile failed nine dependency-floor checks.
These are rendering and dependency regression checks, not exploitability tests.

## Math rendering and sanitizer alignment

Upgrading only the direct KaTeX package changes its imported CSS but can leave
`rehype-katex` using a nested older renderer. KaTeX 0.18 introduced prefixed
classes such as `katex-base`; its old renderer emitted `base`. Real Markdown
rendering tests reproduced the mismatch for inline/display fractions, square
roots, and aligned equations during PR #72 preparation.

All KaTeX copies remain aligned at the 0.19 release line. This is an intentional,
compatibility-tested cross-minor override for a pre-1.0 package, not an assumption
that every 0.x minor is interchangeable. The missing-font-metrics test also
checks that the default strict policy remains non-fatal. Keep the renderer and
stylesheet versions aligned when updating the override.

Direct and nested DOMPurify copies have a 3.4.16 minimum, including Mermaid's
sanitizer. Tests cover active HTML removal, SVG geometry, KaTeX classes and
accessible MathML. DOMPurify's default annotation removal is retained. No
sanitizer allowlist is relaxed. Malformed math has a readable fallback, and
untrusted math cannot create executable links.

## Request and rendering lifecycles

Vitest 4.1.11 originally exposed a version-check request still running after
unmount: all assertions passed, but late logging failed worker teardown. The
hook aborts effect-scoped requests and checks cancellation after asynchronous
response/storage reads. Routing tests use deterministic network responses and
unmount before restoring globals. These guards remain active under Vitest 5.

The stable ESLint/React upgrade also validates effect lifetimes. CV content and
history requests ignore cancelled results, initial CV requests use the available
authentication token, and portal targets are held in state rather than read
from refs during render. Copy-feedback keys change only on a copy event.
Dataset and prompt-shortcut loading discard stale results. Search navigation
uses a cancellable animation frame, and pagination/filter bounds are reconciled
without cascading effects. Storage quota errors preserve the underlying cause.

Full-project ESLint, unit tests, TypeScript, and the production Vite build are
required CI gates. Backend browser/PDF integration and the production image
build are also required. No test-error reporting or validation gate is disabled.
