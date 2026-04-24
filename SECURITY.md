# Security

## Supported Versions

Security fixes target the current `main` branch unless a release branch is
created in the future.

## Reporting a Vulnerability

Please do not open a public issue for a suspected vulnerability. Use GitHub
private vulnerability reporting if it is enabled for the repository. If it is
not enabled, contact the repository owner through their GitHub profile and
avoid including exploit details in public comments.

## Deployment Security

This project is intended for self-hosted use.

- Replace `AUTH_PASSWORD` and `AUTH_SECRET` before exposing the app.
- Expose only the web app port. Keep the API port internal to Docker Compose.
- Put the app behind HTTPS, a VPN, or another trusted access layer when it is
  reachable outside localhost.
- Treat the media directory as sensitive. It may contain file names, logs,
  runtime settings, and saved processing pipelines.
- Do not commit `.env`, media files, logs, generated binaries, or local agent
  configuration.

## Known Security Posture

The web app uses a simple single-password session cookie. It is not a full
multi-user authentication system and does not provide per-user authorization.
Use an external identity-aware proxy if you need stronger access control.
