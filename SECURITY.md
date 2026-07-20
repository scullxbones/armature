# Security Policy

## Reporting a Vulnerability

Armature takes security seriously. If you discover a security vulnerability in Armature, please report it responsibly by following these steps:

### Responsible Disclosure

1. **Do not open a public issue** for security vulnerabilities
2. **Report via GitHub** — Use [GitHub's private vulnerability reporting](https://github.com/scullxbones/armature/security/advisories/new) (Security tab -> "Report a vulnerability") with:
   - Description of the vulnerability
   - Steps to reproduce (if applicable)
   - Potential impact — for Armature this includes the harness hook that mediates agent tool calls, since a bypass there affects scope enforcement for every AI contributor
   - Your contact information
3. **Coordinated disclosure** — We will work with you to determine an appropriate disclosure timeline before publishing any security announcements

### Security Advisory Process

We use GitHub Security Advisories to publish security updates. When a vulnerability is confirmed:

1. We will develop and test a fix
2. We will coordinate with you on the disclosure timeline
3. We will publish a security advisory on GitHub with:
   - Description of the vulnerability
   - Affected versions
   - Fixed versions
   - Workarounds (if applicable during the fix window)
   - Credit to the reporter (with your permission)

### Supported Versions

Security updates are released for the latest version. We recommend always running the latest release of Armature.

### Additional Security Information

- [CONTRIBUTING.md](CONTRIBUTING.md) — Contribution guidelines and code quality standards
- [docs/design/architecture.md](docs/design/architecture.md) — System architecture and design
- [CONSTITUTION.md](CONSTITUTION.md) — Core invariants and principles

Thank you for helping keep Armature secure.
