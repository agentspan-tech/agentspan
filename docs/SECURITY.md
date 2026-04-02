# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |

## Reporting a Vulnerability

**Please do NOT open a public GitHub issue for security vulnerabilities.**

Use GitHub's private vulnerability reporting feature:

1. Go to the repository's **Security** tab
2. Click **Advisories** > **Report a vulnerability**
3. Fill in the details as described below

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected component (proxy, processing, frontend, Docker configuration)
- Impact assessment (what an attacker could achieve)
- Any suggested fix (optional)

### Response Timeline

- **Acknowledgment:** Within 48 hours of report
- **Initial assessment:** Within 5 business days
- **Fix timeline:** Based on severity:
  - Critical: patch within 7 days
  - High: patch within 14 days
  - Medium: next scheduled release
  - Low: best effort

### Scope

**In scope:**
- AgentSpan proxy service
- AgentSpan processing service
- Frontend dashboard
- Docker Compose configuration
- Authentication and authorization logic
- API key encryption and storage
- Provider API key handling

**Out of scope:**
- Third-party dependencies (report directly to the upstream project)
- Denial of service via resource exhaustion (covered by Docker resource limits)
- Issues requiring physical access to the host

## Security Architecture

AgentSpan handles sensitive data (LLM provider API keys, user credentials). Key security measures:

- **API keys**: Stored as HMAC-SHA256 digests; raw keys never persisted
- **Provider keys**: Encrypted at rest with AES-256-GCM with key versioning
- **Passwords**: bcrypt with SHA-256 prehash to prevent 72-byte truncation
- **JWT**: HS256 with algorithm validation to prevent algorithm-switching attacks
- **CSRF**: `X-Requested-With` header check on mutating cookie-authenticated requests
- **CSP**: Content Security Policy with script-src 'self' and frame-ancestors 'none'

## Known Limitations

- **CSP `style-src 'unsafe-inline'`**: Required by React inline styles and UI component libraries (Radix UI, shadcn). All inline styles are hardcoded or computed from layout -- none use user-controlled data. React's JSX auto-escaping prevents injection. A nonce-based approach is planned for a future release.
- **JWT invalidation**: JWTs are stateless; changing a password invalidates new tokens via `password_changed_at` check, but existing tokens remain valid until expiry. Keep `JWT_TTL_DAYS` reasonable.
