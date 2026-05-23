# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| 1.x     | ✅ Active support  |

## Reporting a Vulnerability

**DO NOT** create a public GitHub issue for security vulnerabilities.

Contact the security team directly:
- Discord: **@rimix.98** (primary) or **@.samoletik** (admin)
- Telegram: **@samoltik**
- GitHub: Open a [draft security advisory](https://github.com/<owner>/SMLT-Demonlist/security/advisories/new)

We aim to acknowledge reports within 48 hours and deploy a fix within 7 days.

## Security Architecture

### Authentication
- **Password**: Stored as bcrypt hash (cost 10), never in plaintext.
- **JWT**: HS256-signed, 24h expiry, `jti` claim for individual revocation, `ver` claim for bulk invalidation.
- **Cookies**: `HttpOnly`, `Secure`, `SameSite=Strict`. Auth and CSRF tokens are separate cookies.
- **CSRF**: Double-submit cookie pattern. Token in HttpOnly cookie + `X-CSRF-Token` header.

### Rate Limiting
- **All endpoints**: 30–60 requests/min per IP (sliding window).
- **Login endpoint**: 5 requests/min per IP with CAPTCHA requirement.
- **Production**: Upstash Redis (distributed). **Fallback**: In-memory (per-Vercel-instance).
- **IP extraction**: Trusted headers from Vercel (`X-Vercel-Forwarded-For`) with validated leftmost `X-Forwarded-For` fallback.

### Input Validation
- All JSON bodies: limited to 1 MB, unknown fields rejected.
- All string inputs: sanitized via `bluemonday` UGCPolicy.
- Project IDs, video IDs, nicknames, Discord tags, role names, hex colors: regex-validated.
- No `innerHTML` or raw HTML attribute injection in frontend.

### Database (Firestore)
- Server-side writes: only through authenticated, CSRF-protected endpoints.
- Transactions used for all read-modify-write operations (staff roles).
- Batch writes for project saves use `MergeAll` to prevent field loss.
- Token blacklist collection (`token_blacklist`) with 24h TTL for forced logout.

### Frontend Security
- No `innerHTML`, `outerHTML`, `insertAdjacentHTML`, or `document.write`.
- All text content set via `textContent` or `document.createTextNode`.
- Attribute values set via `setAttribute()` with proper escaping.
- Styles set via `style` property (not `cssText`).
- All external links use `rel="noopener noreferrer"`.
- CSP enforced: `default-src 'self'`, `frame-src https://www.youtube.com`, `form-action 'self'`.

### Data Collection
See [PRIVACY.md](PRIVACY.md) or the website's info modal for details on what we collect.
We do NOT store IP addresses, browser fingerprints, or personal identifiable information beyond Discord tags and in-game nicknames.

## Deployment Checklist

Before deploying to production:
1. [ ] `JWT_SECRET` is ≥32 random characters
2. [ ] `ADMIN_HASH` is a bcrypt hash of a strong password
3. [ ] `FIREBASE_CREDENTIALS` is the service account JSON (single line)
4. [ ] `UPSTASH_REDIS_REST_URL` and `UPSTASH_REDIS_REST_TOKEN` are set
5. [ ] `VERCEL=1` is set in production (automatic on Vercel)
6. [ ] Firestore security rules restrict client access (server-only writes)

## Known Security Considerations

1. **Demonlist.org dependency**: Leaderboard data is fetched from an external API. We validate the response schema but cannot guarantee data integrity from the upstream.
2. **JWT token lifetime**: Tokens expire after 24 hours. For immediate revocation, increment `tokenVersion` in Firestore (`config/auth`).
3. **No user registration**: The admin panel is accessible only via shared password. Audit logging tracks all admin actions.
