# BasaltPass Tools

This directory is for local development, test data, and one-off helper scripts.

- `dev/`: manual development helpers that are not part of the stable startup flow.
- `conformance/`: OpenID conformance helper scripts.

Stable full-stack entrypoints stay in `../scripts/`, such as `dev.sh` and `dev.ps1`.

Backend tools that import `basaltpass-backend/internal` packages stay under
`../basaltpass-backend/tools/` so Go internal package visibility still works.
