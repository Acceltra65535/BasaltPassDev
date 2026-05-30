# BasaltPass Roadmap

This roadmap describes the current direction for BasaltPass. It is not a
guarantee of delivery dates or final scope.

## Current Focus

- Make local development and verification reproducible for new contributors.
- Keep OAuth 2.0 and OIDC behavior aligned with implemented capabilities.
- Improve documentation for integration, deployment, and security operations.
- Clarify the public API surface and configuration model before stable releases.
- Reduce repository noise by keeping generated files and local runtime artifacts
  out of source control.

## Near Term

- Add root-level verification scripts for backend, frontend, and docs.
- Strengthen CI checks for backend tests, frontend build, and documentation
  build.
- Document supported OAuth/OIDC flows, unsupported flows, and compatibility
  expectations.
- Review tracked files and remove generated frontend build output from version
  control.
- Add clearer examples for local `.env` setup without committing secrets.
- Improve contributor onboarding documentation.

## Backend

- Expand tests around OAuth/OIDC token lifecycle, PKCE, nonce handling, JWKS,
  key rotation, introspection, revocation, and logout.
- Expand tests around tenant isolation, RBAC boundaries, and S2S authorization.
- Clarify migration strategy for SQLite development and MySQL production.
- Review configuration defaults so production deployments fail closed when
  required secrets are missing.
- Keep security-sensitive behavior documented near the code and public docs.

## Frontend

- Add explicit typecheck, lint, and test commands.
- Continue consolidating UI around shared design-system components.
- Reduce large production chunks through route-level lazy loading and chunk
  strategy review.
- Improve consistency across user, tenant, and admin consoles.
- Keep generated build metadata and `dist/` output out of source control.

## Documentation

- Keep `README.md` focused on quick start, architecture, and project status.
- Move detailed integration material into `basaltpass-docs/docs/`.
- Keep deployment docs current for Docker Compose, image-based deployment, and
  Kubernetes-style environments.
- Add compatibility notes for SDKs and example apps.
- Maintain a changelog for user-visible changes.

## Release Readiness

Before a stable public release, the project should have:

- Reproducible local test commands.
- Clean CI on a fresh clone.
- No committed secrets, local databases, logs, or generated frontend output.
- Clear security reporting and support policy.
- Documented supported OAuth/OIDC behavior.
- A defined versioning and release process.
- A documented SDK publication strategy.

