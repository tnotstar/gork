# Security Profile - gork

This document defines the security parameters, threat profile, and contractual compliance controls for `gork`.

## STRIDE Threat Matrix

| Threat Category | Threat Description | Mitigation Strategy |
| :--- | :--- | :--- |
| **Spoofing** | Masquerading targets or spoofing client IPs | Only support standard TCP/IP dials without custom raw socket injection. Validate host schemes prior to resolution. |
| **Tampering** | Buffer overflows in custom state machine | Implement strict boundaries on buffer sizing in `htparser`. Truncate excessively long headers. |
| **Repudiation** | benchmarker used to run unlogged attacks | Track benchmark invocations with audit logs. Require logging of target domains. |
| **Information Disclosure** | Leak of remote server headers carrying auth tokens | Redact standard Authorization and Cookie headers from debugging or parsing outputs. |
| **Denial of Service** | Exhaustion of local system file descriptors (FD) | Enforce safety caps on requested concurrency `-c` relative to active OS `ulimit` bounds. |
| **Elevation of Privilege** | Exploiting raw socket capabilities | Run exclusively in user mode without requiring root or network capabilities (`CAP_NET_RAW`). |

## Compliance Controls (C1 - C11)

### C1: Secure Transport
- Support HTTPS targets utilizing TLS 1.3. Reject SSLv3/TLS 1.0 negotiations.

### C2: Rate Limiting
- Not applicable; the explicit goal of the application is controlled load testing.

### C3: Authentication & Token Security
- Support header insertion for target authorizations. Safely zero memory segments carrying sensitive authentication header values upon completion.

### C4: Authorization
- The tool must only be executed against authorized target domains. Unsanctioned load injection is forbidden.

### C5: Identifiers
- Benchmark run reports must generate a unique run ID using UUIDv7.

### C6: Request Hygiene
- Enforce strict read/write timeouts on Event Loop sockets. Set maximum limits on incoming HTTP header block sizes (default 8KB) to prevent OOM.

### C7: Containment & Runtime
- When containerized, the runtime image must be built on scratch or distroless configurations, executing as a non-privileged user.

### C8: Secrets Management
- Standard CLI headers containing API keys or bearer tokens must be passed via environment variables or file inputs, never hardcoded in configurations.

### C9: Structured Logging
- Application lifecycle state changes (start, progress, teardown) must utilize structured JSON logging via `log/slog`. No client token details may be leaked.

### C10: Supply Chain & CI/CD
- Pinned external dependencies (like `gnet/v2` and `bytebufferpool`). Vulnerability audits performed regularly via Trivy.

### C11: Graceful Teardown
- Trap `SIGTERM` and `SIGINT`. Upon signal interception, gracefully drain active connection buffers, print generated metrics, close file descriptors, and terminate event loops cleanly.
