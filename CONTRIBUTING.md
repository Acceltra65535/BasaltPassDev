# Contributing to BasaltPass

Thank you for your interest in contributing to BasaltPass.

BasaltPass is an identity and access platform, so changes should be made with
extra care around security, tenant isolation, OAuth/OIDC correctness, and
backward compatibility.

## Project Status

BasaltPass is under active development. Public APIs, configuration keys, and
deployment patterns may still change before a stable release.

Before proposing large changes, please open an issue or discussion describing
the problem, expected behavior, and implementation direction.

## Repository Layout

- `basaltpass-backend/`: Go backend API and domain services.
- `basaltpass-frontend/`: React + TypeScript user, tenant, and admin consoles.
- `basaltpass-docs/`: Docusaurus documentation site.
- `docs/`: project-level deployment, route, and planning documents.
- `scripts/`: local development helpers.
- `test/`: project-level verification scripts and API checks.

## Development Setup

Use Docker Compose for a full local stack:

```bash
docker compose --profile localdb up -d --build
```

Or use the native development scripts:

```bash
./scripts/dev.sh up
./scripts/dev.sh status
```

On Windows PowerShell:

```powershell
.\scripts\dev.ps1 up
.\scripts\dev.ps1 status
```

Never commit local secrets, runtime databases, logs, generated build output, or
machine-specific editor settings.

## Backend

Backend code lives in `basaltpass-backend` and uses Go, Fiber, and GORM.

Run tests with an isolated test environment so local `.env` settings do not
affect results:

```bash
cd basaltpass-backend
JWT_SECRET=test-secret-for-local \
BASALTPASS_ENV_FILE=/dev/null \
BASALTPASS_DATABASE_DRIVER=sqlite \
BASALTPASS_DATABASE_DSN='file:basaltpass-test?mode=memory&cache=shared' \
go test ./...
```

On Windows PowerShell:

```powershell
cd basaltpass-backend
$env:JWT_SECRET='test-secret-for-local'
$env:BASALTPASS_ENV_FILE='NUL'
$env:BASALTPASS_DATABASE_DRIVER='sqlite'
$env:BASALTPASS_DATABASE_DSN='file:basaltpass-test?mode=memory&cache=shared'
go test ./...
```

Backend contributions should include tests when they affect authentication,
authorization, tenant isolation, token lifecycle, payment state, or public API
behavior.

## Frontend

Frontend code lives in `basaltpass-frontend` and uses npm workspaces.

```bash
cd basaltpass-frontend
npm ci
npm run build
```

For UI work, prefer the shared design-system components documented in
`basaltpass-frontend/CONTRIBUTING.md`. Avoid introducing one-off raw controls
when an existing `P*` component covers the use case.

## Documentation

Documentation changes should be kept close to the feature they describe:

- Product and integration docs belong in `basaltpass-docs/docs/`.
- Deployment and project planning docs belong in `docs/`.
- Backend-specific operational details may also be documented in
  `basaltpass-backend/README.md`.

## Pull Request Checklist

Before opening a pull request:

- Keep the change focused on one problem.
- Include tests or explain why tests are not practical for the change.
- Run the relevant backend and frontend verification commands.
- Update docs when behavior, configuration, or public APIs change.
- Do not include secrets, local databases, logs, `dist/`, or dependency folders.
- Call out any security-sensitive behavior in the PR description.

## Security

Do not report security vulnerabilities through public issues. Follow
`SECURITY.md` and use GitHub Private Vulnerability Reporting when available.

