# Frontend dependency compatibility

## Reproducible commands

Use Node.js 24, as used by CI and the production Docker build, and enable
Corepack. The repository root and `web/package.json` both pin the same exact
pnpm release. The root manifest exists because Corepack selects its executable
before pnpm processes `-C web`; omitting the root pin can select an incompatible
pnpm major on a fresh machine.

From the repository root:

```sh
corepack enable
pnpm -C web install --frozen-lockfile
pnpm -C web test
pnpm -C web build
```

Update both package-manager pins together. Their regression test checks that they
remain identical. Peer-dependency validation is enabled in `web/.npmrc`.

## Dependency update policy

The October 2026 reconciliation incorporates dependency PRs #44, #47, #52, #53,
#55, #57, #61, #62, and #64 on the current application code rather than merging
nine stale copies of the same lockfile. Direct dependency minimums are raised;
conditional transitive overrides stay within their existing major versions and
allow newer compatible patches. Avoid exact-version overrides for peer ranges,
which can reject a newer compatible package already selected elsewhere.

Generate the lockfile with pnpm, then format it with the repository's Prettier
configuration. Do not edit integrity hashes manually. Dependency-floor tests
check the resolved package section, including transitive PostCSS copies, and
preserve the existing DOMPurify minimum. A dependency removed entirely from the
transitive graph does not need to be reintroduced solely to satisfy an old PR.

## Mermaid compatibility

Mermaid 11.15 changes dotted class-diagram namespaces into hierarchical groups
by default. Existing chat diagrams use flat dotted labels such as `api.v1`.
`createMermaidConfig` explicitly disables automatic hierarchical namespaces to
preserve those diagrams, while retaining strict security and both color themes.
Tests exercise the real Mermaid parser and layout data, valid flowcharts and
sequence diagrams, and malformed-input rejection.

Removing the compatibility setting reproduces failures in both namespace-theme
tests. Restoring the pre-update lockfile reproduces nine dependency-floor
failures. These negative controls were verified during preparation, followed by
the passing fixed tests. They are not tests of vulnerability exploitability.

Vite and Vitest configurations use `import.meta.dirname` rather than CommonJS
`__dirname`, so the path aliases also work with native ESM configuration loading.
The normal PR pipeline remains the final merge gate for tests, formatting,
lint, type checking, backend checks, and the production image build.
