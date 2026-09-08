---
name: gumroad-cli-pp
description: Use when maintaining or verifying the independent Printing Press project that consumes antiwork/gumroad-cli.
---

# Gumroad CLI PP

This project uses the upstream Gumroad CLI intact. Read `PP_PROVENANCE.md` before changing remotes, importing upstream work, or modifying project-owned files.

## Boundaries

- Keep `origin` for `keithah/gumroad-cli-pp` and `upstream` for `antiwork/gumroad-cli`.
- Do not replace, fork, or reimplement upstream command behavior in PP-owned files.
- Use `GUMROAD_ACCESS_TOKEN` only as a process-local environment variable for explicitly authorized, read-only acceptance checks. Never persist or print credentials, response bodies, or customer data.
- Merchant writes, uploads, package releases, deployments, and upstream pushes require explicit approval.

## Verification

Run the upstream suite before publishing PP-owned changes:

```bash
make test-cover
make test-race
make lint
go build ./cmd/gumroad
```

For a live read-only check, retrieve the approved seller token only from 1Password, pass it through `GUMROAD_ACCESS_TOKEN`, parse only the required response shape/counts, and delete temporary output before reporting.
