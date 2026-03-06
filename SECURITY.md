# Security Policy

## Reporting a Vulnerability

**Do not open a public issue for security vulnerabilities.**

Please report vulnerabilities through [GitHub Security Advisories](https://github.com/gerty-labs/gerty/security/advisories/new).

## Scope

This policy covers the public Gerty repository:

- Agent (DaemonSet collector)
- CLI (`gerty` kubectl plugin)
- Helm chart (`deploy/helm/gerty/`)

**Out of scope:** gerty-core (server, rules engine, SLM) is a separate private repository with its own security process.

## Response Timeline

| Step | Target |
|------|--------|
| Acknowledgement | 48 hours |
| Initial assessment | 5 business days |
| Fix for critical severity | 30 days |

## Disclosure

We follow coordinated disclosure. Vulnerabilities will not be disclosed publicly until a patch is available. We will credit reporters in the release notes unless they prefer to remain anonymous.
