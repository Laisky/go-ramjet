# Arweave archive alert diagnostics

## Incident and root cause

The 2026-11-18 16:31:38 UTC alert `failed to fetch and save url content` carried `{}`. A bounded, read-only container-log check for 16:31:30 through 16:31:45 found the matching failure: fetch -> submit crawler task -> admit crawler target -> non-public addresses refused. The target was an HTTP non-global IP literal with a query. The target itself is intentionally omitted. Central-log searches returned no matching rows; the incident conclusion comes from the container log, not the central index.

The refusal is expected destination-policy behavior, not evidence of a provider outage or storage failure. No target request is needed or authorized to reproduce it. The distinct notification defect is that the alert core exports fields only for the reviewed verified-crawler logger. The archive worker used the root logger, so its URL/error fields were intentionally removed by that privacy boundary. Prior crawler fixes do not cover this scope.

## Correction

Use a distinct archive logger and an intentionally bounded projection of stage, category, elapsed milliseconds and a SHA-256 correlation digest of the exact target. Raw URLs, raw errors, arbitrary fields and inherited logger context remain outside outbound archive alerts; ordinary local logging retains its existing fields. Other logger scopes remain metadata-only, and reviewed crawler logging remains unchanged.

A typed local admission wrapper preserves the original cause and exposes admission attribution without parsing dependency error strings. `admission_failed` is deliberately broader than private-address denial: DNS/other admission failures must not be mislabeled. Cancellation and timeout identities remain intact. Severity is unchanged because not every admission failure is an expected input refusal.

The archive per-item function was extracted for the actual source-path regression. Fetching, storage, the three-minute budget and existing duplicate suppression are unchanged. This does not fix or certify the existing separate Load/Store suppression race.

## Validation

- RED: real pinned SDK HTTP export reproduced the exact empty `{}` before correction.
- GREEN: real archive per-item worker -> fetch -> loopback destination rejection -> alert core -> HTTP export emits safe admission metadata; the target server receives no request.
- Race-enabled logging, archive source-path, admission/error-identity and existing alert controls pass.
- Full logging suite passes with race detection; wrapped cancellation identity, storage/fetch categories, inherited-field exclusion, duplicate notification and hook/core boundaries are covered.
- Scoped Go vet and golangci-lint pass; import formatting and diff checks pass.

No new dependencies or CI jobs. No live-provider calls, production mutation, deployment or merge. Full repository/browser qualification is not claimed. FileIO historical searches returned unrelated score-zero results; no absence proof is claimed. FileIO closeout export was not attempted because a separate parent attempt had been denied.
