# CV Editing Authorization

The public CV (`GET /cv/content`, `GET /cv/pdf`, `GET /cv/meta`) needs no
session. Every route that reveals drafts or history, or changes or renders
content, requires a Laisky SSO session that belongs to a configured owner:

- `GET /cv/content/history`
- `GET /cv/content/version`
- `PUT /cv/content`
- `POST /cv/pdf/preview`
- `GET /cv/auth/session` (lets the editor confirm the session before enabling itself)

## Flow

1. The editor sends the visitor to `https://sso.laisky.com/?redirect_to=<cv url>`.
2. SSO returns with `?sso_token=<JWT>`. The page moves the token to
   `localStorage` and scrubs it from the address bar before issuing any request.
3. The page calls `GET /cv/auth/session`. Editing is offered only after it
   succeeds; holding a token is not enough.
4. The backend verifies each protected request by calling the SSO GraphQL
   `WhoAmI` query with the bearer token. SSO checks the EdDSA signature, issuer,
   expiry, `sub == uid` and that the account is active. CV then checks that the
   identity SSO returned equals the token's own subject and is listed in
   `tasks.cv.sso.owner_uids`. CV holds no SSO signing material.

## Responses

| Status | `code`                    | Meaning                                                    |
| ------ | ------------------------- | ---------------------------------------------------------- |
| 401    | `sso_session_missing`     | No bearer token.                                           |
| 401    | `sso_session_invalid`     | Malformed, expired, tampered, foreign or inactive session. |
| 403    | `cv_owner_required`       | A valid session for an account that may not edit the CV.   |
| 503    | `sso_unavailable`         | SSO timed out, failed, redirected or answered malformed.   |
| 503    | `cv_owner_not_configured` | `tasks.cv.sso` is missing or invalid.                      |

Malformed tokens are rejected locally and never reach SSO. The verifier uses a
fixed endpoint, never follows redirects (which would forward the bearer
token), reads at most 64 KiB and times out after `tasks.cv.sso.timeout`
(default 5s). Errors and logs never contain the token.

## Configuration

```yaml
tasks:
  cv:
    sso:
      graphql_endpoint: 'https://sso.laisky.com/query' # default; https required
      owner_uids:
        - '<owner SSO UID>'
      timeout: 5s
```

The owner UID is the stable SSO UID returned as `WhoAmI.id` and carried as the
JWT `sub`/`uid`. It is not a secret.

Other routes that use the legacy HS256 middleware (`server.jwt_secret`), such as
the Arweave DNS routes, are unchanged.
