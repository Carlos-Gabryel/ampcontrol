# Security policy

**English** · [Português (Brasil)](SECURITY.md)

## Supported versions

Until the first stable release, security fixes are applied only to the latest version of the main branch. Older installations should update before requesting support.

## Reporting a vulnerability

Do not publish tokens, passwords, internal addresses, personal IDs, or exploitable details in a public issue.

Use **Security > Report a vulnerability** in the GitHub repository to submit a private advisory. Include the version/commit, impact, reproduction conditions, and a minimal proof without real data. If private advisories are unavailable, open a public issue without sensitive details and request a private channel.

## Security model

AmpControl protects against misuse by regular Discord members and reduces local credential exposure. It cannot protect a host whose root account, AMP administrator account, or Discord token is already compromised.

Required practices:

- use a dedicated Discord bot;
- use a dedicated least-privilege AMP account;
- restrict `/ampconfig` to the owner and trusted roles;
- keep the audit channel private;
- do not expose ADS or RCON directly to the internet;
- review logs and backups before sharing them;
- immediately rotate any secret exposed as plaintext.

Host-key-encrypted credentials protect data at rest from casual copying, but root on that host can still access them. This is expected systemd behavior.
