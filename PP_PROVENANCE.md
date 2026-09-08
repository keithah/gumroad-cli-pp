# Printing Press Provenance

This is an independent Printing Press project that consumes the maintained [Gumroad CLI](https://github.com/antiwork/gumroad-cli). It is not a GitHub fork and does not reimplement the Gumroad API client.

## Seed revision

- Upstream repository: `https://github.com/antiwork/gumroad-cli.git`
- Seed commit: `ea126f7044e4cae79f5d63e2f57b6cb4144f90bf`
- Upstream license: MIT

## Remote policy

- `origin` is `https://github.com/keithah/gumroad-cli-pp.git` and is the only publishing remote.
- `upstream` is `https://github.com/antiwork/gumroad-cli.git` and is read-only for this project.
- Upstream updates are imported as explicit, reviewable commits after the upstream test and build gates pass.

## Project boundary

The upstream command surface is consumed unchanged. PP-owned work is limited to provenance, update policy, project guidance, and verification automation. This repository does not publish packages or binaries, make merchant mutations, or push to upstream without separate explicit approval.
