# Sentinel Journal - Critical Security Learnings

## 2026-02-02 - Added Standard Security Headers

**Vulnerability:** Missing standard HTTP security headers (X-Content-Type-Options, X-Frame-Options, Referrer-Policy).
**Learning:** Even if a web server is intended to be behind a proxy like Nginx, it's defense-in-depth to implement security headers at the application level to ensure they are always present.
**Prevention:** Include a standard security middleware in all web server initializations to set these headers by default.
