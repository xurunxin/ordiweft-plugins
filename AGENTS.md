# Plugin repository maintenance

- Keep Apache-2.0 and each package's LICENSE/NOTICE. Morpheus is not redistributed here.
- Use PowerShell 7 without a profile and the workstation RTK proxy. Document original commands.
- Sources, recipes, review records and unsigned catalog proposals are separate. Never install dependencies, execute package code, accept install hooks, read deployment secrets or create signing keys during check/build.
- Package versions and output files are immutable. Changes require a new version; do not rewrite old release bytes, receipts or withdrawals.
- `node scripts/registry.mjs check` checks repository metadata. `build` additionally requires an explicitly supplied trusted Ordiweft CLI and its SHA-256. This packs prebuilt bytes; it is not a TypeScript/npm sandbox builder.
- `prepare` creates an unsigned proposal only. Signing, publishing, enabling and production deployment are separate human-controlled operations. No automatic merge or deployment.
- Validate real packed outputs with the host CLI and cover refusal/immutability before claiming delivery. Keep PASS, FAIL, SKIP and NOT_RUN distinct.
