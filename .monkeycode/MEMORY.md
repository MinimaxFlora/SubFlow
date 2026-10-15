# User Instruction Memory

This file records user instructions, preferences, and teachings for reference in future interactions.

## Format

### User Instruction Entry

[User Instruction Summary]
- Date: [YYYY-MM-DD]
- Context: [Mentioned scenario or time]
- Instructions:
  - [Content of user teaching or instruction, described line by line]

### Project Knowledge Entry

[Project Knowledge Summary]
- Date: [YYYY-MM-DD]
- Context: Discovered by Agent while performing [specific task description]
- Category: [Operations & Deployment|Build Methods|Testing Methods|Troubleshooting & Debugging|Workflow & Collaboration|Environment Configuration]
- Instructions:
  - [Specific knowledge points, described line by line]

## Deduplication Strategy

- Before adding a new entry, check for similar or identical instructions.
- If a duplicate is found, skip the new entry or merge it with the existing one.
- When merging, update the context or date information.
- This helps avoid redundant entries and keeps the memory file tidy.

## Entries

[Project Knowledge Summary]
- Date: 2026-10-15
- Context: Discovered by Agent while building and verifying the SubFlow Go backend, Vue frontend and multi-platform conversion engine
- Category: Build Methods
- Instructions:
  - `make check` runs gofmt check, `go test ./...`, `go vet ./...`, frontend `vue-tsc --noEmit`, the Vite production build and the embedded-binary build; use it as the single verification gate.
  - The frontend must be rebuilt before the Go binary to refresh the embedded `web/dist` (the Makefile `backend` target already depends on `frontend`).

[Project Knowledge Summary]
- Date: 2026-10-15
- Context: Discovered by Agent while validating generated sing-box configs against real client binaries
- Category: Testing Methods
- Instructions:
  - `internal/proxy` ships an opt-in integration test that shells out to a real sing-box binary. Run it with `SINGBOX_BIN=/path/to/sing-box go test ./internal/proxy/ -run TestSingBoxAcceptsGeneratedConfig -v`; without the env var the test skips.
  - Generated configs were verified against sing-box stable v1.14.3 and prerelease v1.15.0-alpha.12 (both `sing-box check` exit 0).
  - sing-box rejects a REALITY client that has no uTLS fingerprint, so the producer always injects one (default `chrome`) for REALITY nodes.
  - Clash/ClashMeta output is verified with the real `mihomo -t -d <dir> -f <file>` command (mihomo shows `test is successful`). GitHub release downloads work; the `mihomo-linux-amd64-*.gz` asset needs `gunzip -c` before `chmod +x`.
  - To exercise the fetch -> parse -> convert pipeline without depending on external hosts, serve a captured payload over localhost and start the server with `SUBFLOW_FETCH_ALLOW_PRIVATE_HOSTS=true` on a spare `SUBFLOW_PORT`.

[Project Knowledge Summary]
- Date: 2026-10-15
- Context: Discovered by Agent while diagnosing a `dial tcp <cdn-ip>:443: i/o timeout` when fetching a Cloudflare-fronted subscription
- Category: Troubleshooting & Debugging
- Instructions:
  - Outbound IPv4 egress from this sandbox can time out for some Cloudflare ranges while IPv6 or another address for the same host still works; verify with `curl -4` / `curl -6` before blaming the application.
  - `internal/fetch` resolves all public addresses for a host and dials them concurrently (Happy Eyeballs style, capped at 4, 6s per attempt), so a host whose first address is blocked is still reached through another address family.
  - When a user reports a raw `dial tcp <ip>:443: i/o timeout`, that is a client-side network reachability problem to the upstream host, not evidence that the generated client config is wrong; confirm the fetch succeeds server-side and state the distinction.

[Project Knowledge Summary]
- Date: 2026-10-15
- Context: Discovered by Agent while setting up build and container workflows
- Category: Environment Configuration
- Instructions:
  - Docker is not available in this environment, so `make docker-build`, `make docker-up` and `make docker-test` cannot run locally.
  - The Go module pins `modernc.org/sqlite`, which requires go>=1.26.0; the local toolchain auto-switches to a newer Go version for this module.

[Project Knowledge Summary]
- Date: 2026-10-15
- Context: Discovered by Agent while running `make check`, which failed at the frontend typecheck step
- Category: Environment Configuration
- Instructions:
  - `web/package.json` pins `packageManager: pnpm@11.0.9`, so corepack resolves pnpm per-directory. The first run in a fresh environment may fail while corepack downloads that version; set `COREPACK_ENABLE_DOWNLOAD_PROMPT=0` and re-run `make check` once the tarball is cached.
  - The system `pnpm` shim was broken (corepack expected `bin/pnpm.cjs` while the package shipped `bin/pnpm.mjs`); pnpm 11.28.6 is installed at `/opt/pnpm-global/bin/pnpm` if the shim misbehaves.
  - Building the frontend needs extra heap on this host (`NODE_OPTIONS=--max-old-space-size=2048`) and should run under a managed background terminal with a memory cap (~1.8 GiB peak observed).
