# Backend Tools

Backend-local development tools live here when they need to import
`basaltpass-backend/internal` packages.

- `conformance-seed/`: seeds the local SQLite database used by OIDC conformance runs.
- `create-test-subscription/`: creates subscription product, plan, and price data for local payment testing.

Run them from `basaltpass-backend`, for example:

```bash
go run ./tools/conformance-seed
go run ./tools/create-test-subscription
```
