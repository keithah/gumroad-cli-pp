# Gumroad Code Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a single-tool, JavaScript Code Mode MCP server that reaches every existing public Gumroad CLI operation while requiring `confirm: true` for every mutation.

**Architecture:** Extract the current generated-MCP command discovery, schema, validation, authentication, and in-process execution logic into a reusable command catalog. Build `internal/cmd/codemode` on that catalog with a fresh `goja` runtime per request; it creates a `gumroad` JavaScript object whose methods can invoke catalog operations or return catalog help. Preserve `gumroad mcp` unchanged, then replace Hermes’s nine-tool generated-MCP filter with the one Code Mode server only after its local and live read-only acceptance tests pass.

**Tech Stack:** Go 1.25.4; Cobra; `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0; `github.com/dop251/goja` v0.0.0-20260906210903-70ad66ec7ce4; existing Gumroad config and testutil packages.

**Spec:** `docs/superpowers/specs/2026-09-08-gumroad-codemode-design.md`

## Global Constraints

- Preserve every current `gumroad mcp` command and its 100-tool external behavior.
- Exclude the same command groups as the generated MCP server: `admin`, `auth`, `completion`, `help`, `mcp`, and `skill`.
- Never accept shell text, filesystem access, network access outside the selected CLI operation, process access, or credentials from JavaScript.
- Treat only operations with the current generated `ReadOnlyHint` as reads; classify every other operation as a mutation.
- A mutation must return a non-executed plan unless its second JavaScript argument is exactly `{confirm: true}`.
- Keep secret values out of source, fixtures, error output, logs, documentation examples, and Hermes config.
- Bound JavaScript execution and serialized output. Report timeout and oversize outcomes structurally; never silently truncate JSON.
- Use test-first development. Run focused tests after each task and the full `make test-cover` plus `make lint` suite before changing Hermes configuration.
- Do not release, publish, submit upstream, or write merchant data during acceptance verification.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/cmd/mcp/catalog.go` | Shared immutable public-command catalog, operation metadata, argument decoding, and in-process execution used by both MCP implementations. |
| `internal/cmd/mcp/mcp.go` | Retains the generated 100-tool server and delegates its discovery/execution path to `Catalog`. |
| `internal/cmd/mcp/catalog_test.go` | Contract tests for catalog completeness, metadata, schema, and safe command argument conversion. |
| `internal/cmd/codemode/codemode.go` | Registers the `codemode` Cobra command and starts the one-tool MCP stdio server. |
| `internal/cmd/codemode/server.go` | Defines the one `gumroad` MCP tool, typed input schema, error envelope, result limits, and server construction. |
| `internal/cmd/codemode/runtime.go` | Creates a fresh constrained Goja runtime, registers nested `gumroad` methods/help, applies execution deadline, and serializes structured results. |
| `internal/cmd/codemode/codemode_test.go` | In-memory MCP tests for enumeration, coverage, read/write confirmation, validation, runtime isolation, output bounding, and errors. |
| `internal/cmd/root.go` | Registers `codemode.NewCodeModeCmd(NewRootCmd)` alongside the existing `mcp` command. |
| `go.mod`, `go.sum` | Pin Goja at the version in this plan. |
| `skills/gumroad/SKILL.md` | Documents the one-tool Code Mode entry point and confirmation syntax, as required by upstream contribution guidance. |
| `/Users/hermes/work/bin/gumroad-pp-mcp` | Becomes a local wrapper for `gumroad-cli-pp codemode`, without a `--dry-run` global override because per-call confirmation is authoritative. |
| `~/.hermes/config.yaml` | Replaces the generated-MCP nine-tool include list with only the Code Mode `gumroad` tool after successful verification. This is an external configuration change and must be read back. |

## Task 1: Extract the shared public command catalog

**Files:**
- Create: `internal/cmd/mcp/catalog.go`
- Create: `internal/cmd/mcp/catalog_test.go`
- Modify: `internal/cmd/mcp/mcp.go`
- Modify: `internal/cmd/mcp/mcp_test.go`

**Interfaces:**
- Produces `type Operation struct { Name string; Path []string; Schema map[string]any; ReadOnly bool; Command *cobra.Command; Flags *pflag.FlagSet }`.
- Produces `type Catalog struct` with `NewCatalog(newRoot func() *cobra.Command) *Catalog`, `Operations() []Operation`, `Operation(name string) (Operation, bool)`, `Help(name string) (any, error)`, and `Execute(ctx context.Context, operation Operation, raw json.RawMessage) (json.RawMessage, error)`.
- `Catalog.Execute` must continue to resolve auth, reject unavailable stdin, set `--json --no-input --quiet`, set `--yes` only when the operation supports it, and reject unknown/wrongly typed flags before command execution.
- The existing `NewServer(newRoot)` must use a `Catalog` and preserve generated tool names, schemas, output, annotations, and error semantics.

- [ ] **Step 1: Write catalog completeness and compatibility tests**

Add tests that build both `NewCatalog(cmd.NewRootCmd)` and `mcpcmd.NewServer(cmd.NewRootCmd)`. Assert that catalog operation names equal the generated MCP tool names exactly, the count is 100, excluded namespaces do not occur, `products_list` has `ReadOnly == true`, and `products_create`, `licenses_verify`, and `sales_refund` have `ReadOnly == false`.

Add a `testRoot` fixture assertion that calls `Catalog.Execute` for `show` using a raw JSON argument object and verifies the same `args`, typed flags, forced `yes`, and unavailable stdin behavior currently asserted by `TestFlagTypesAndExclusions`.

- [ ] **Step 2: Run the new tests to verify RED**

Run: `go test ./internal/cmd/mcp -run 'TestCatalog(Completeness|Execute)' -count=1`

Expected: FAIL because `Catalog`, `NewCatalog`, and `Catalog.Execute` do not exist.

- [ ] **Step 3: Implement immutable operation discovery**

Create `catalog.go`. Move `walk`, `excludedCommand`, `commandFlags`, `flagType`, `commandTool` schema construction, `commandArgs`, `flagValues`, `decodeValue`, `unavailableInput`, and the common command execution path out of `mcp.go` into catalog-owned code.

`NewCatalog` must create a fresh root only for discovery, copy each command path and flag set into an operation map, sort operation names for deterministic `Operations()` and help output, and derive `ReadOnly` from the same annotation logic currently used by `commandTool`.

Implement `Execute` to obtain a fresh root for every call, construct the same CLI argv from the saved operation metadata, install buffered stdout/stderr and `unavailableInput`, run `ExecuteContext`, and return validated JSON output. If a command emits non-JSON despite forced `--json`, return a typed safe error rather than returning a text blob.

Update `NewServer` to iterate catalog operations and call `Catalog.Execute`; it must keep its existing `ToolAnnotations`, descriptions, and `toolResult` behavior.

- [ ] **Step 4: Run focused MCP tests to verify GREEN**

Run: `go test ./internal/cmd/mcp -count=1`

Expected: PASS, including the new catalog tests and all existing generated-MCP compatibility tests.

- [ ] **Step 5: Run formatting and commit the isolated refactor**

Run: `gofmt -w internal/cmd/mcp/catalog.go internal/cmd/mcp/catalog_test.go internal/cmd/mcp/mcp.go internal/cmd/mcp/mcp_test.go && go test ./internal/cmd/mcp -count=1`

Commit:
```bash
git add internal/cmd/mcp/catalog.go internal/cmd/mcp/catalog_test.go internal/cmd/mcp/mcp.go internal/cmd/mcp/mcp_test.go
git commit -m "refactor: share Gumroad MCP command catalog"
```

## Task 2: Add the Code Mode MCP command and one-tool server

**Files:**
- Create: `internal/cmd/codemode/codemode.go`
- Create: `internal/cmd/codemode/server.go`
- Modify: `internal/cmd/root.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `internal/cmd/codemode/codemode_test.go`

**Interfaces:**
- Produces `func NewCodeModeCmd(newRoot func() *cobra.Command) *cobra.Command` with command name `codemode`.
- Produces `func NewServer(newRoot func() *cobra.Command) *sdk.Server` that registers exactly one tool named `gumroad`.
- Tool input contract: `{"code": "string"}` with no other properties.
- Tool output is one JSON text content object. Success is `{"ok":true,"value":...}`; failure is `{"ok":false,"error":{"code":"...","message":"..."}}`.

- [ ] **Step 1: Write failing one-tool protocol tests**

Create `codemode_test.go` with local equivalents of the existing in-memory `connect`, `listTools`, and `call` helpers. Add `TestEnumerationExposesOnlyGumroadTool`: initialize `codemode.NewServer(cmd.NewRootCmd)`, list tools, assert the map contains exactly `gumroad`, assert its input schema has only required string property `code`, and assert `ReadOnlyHint` is absent because the one tool can perform writes.

Add `TestStdioCommandInProcess`: execute a root created by `cmd.NewRootCmd()` with args `codemode`, connect over pipes, and assert only `gumroad` is listed before close/EOF shuts it down.

- [ ] **Step 2: Run the new protocol tests to verify RED**

Run: `go test ./internal/cmd/codemode -run 'Test(EnumerationExposesOnlyGumroadTool|StdioCommandInProcess)' -count=1`

Expected: FAIL because the `internal/cmd/codemode` package and root command registration do not exist.

- [ ] **Step 3: Add Goja and the command/server skeleton**

Run:
```bash
go get github.com/dop251/goja@v0.0.0-20260906210903-70ad66ec7ce4
go mod tidy
```

Implement `NewCodeModeCmd` with the same stdio transport and update-check suppression pattern as `mcp.NewMcpCmd`. In `server.go`, make `NewServer` create the shared catalog and add exactly one manually defined `sdk.Tool` named `gumroad` with the `code` schema. Its handler may temporarily return the safe `runtime_unavailable` error while Task 3 implements execution.

Register the command in `internal/cmd/root.go` and add a root help example:
```text
# Run one compact Gumroad Code Mode MCP tool
gumroad codemode
```

- [ ] **Step 4: Run the protocol tests to verify GREEN**

Run: `go test ./internal/cmd/codemode -run 'Test(EnumerationExposesOnlyGumroadTool|StdioCommandInProcess)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the server boundary**

Run: `gofmt -w internal/cmd/codemode internal/cmd/root.go && go test ./internal/cmd/codemode -count=1`

Commit:
```bash
git add go.mod go.sum internal/cmd/codemode internal/cmd/root.go
git commit -m "feat: add Gumroad Code Mode MCP server"
```

## Task 3: Implement the constrained JavaScript runtime and help API

**Files:**
- Create: `internal/cmd/codemode/runtime.go`
- Modify: `internal/cmd/codemode/server.go`
- Modify: `internal/cmd/codemode/codemode_test.go`

**Interfaces:**
- Produces `type Runtime struct { catalog *mcp.Catalog; timeout time.Duration; maxResultBytes int }`.
- Produces `func (r Runtime) Execute(ctx context.Context, source string) (any, error)`.
- Produces `func (r Runtime) Help(name string) (any, error)`.
- The JavaScript global surface is exactly `gumroad`; it has namespaces for catalog operations and `help(name?)`.
- A callable signature is `gumroad.namespace.operation(arguments?, options?)`. Only `options.confirm === true` confirms a mutation.

- [ ] **Step 1: Write failing runtime API tests**

Add tests with the mock HTTP server:

1. `TestHelpCoversCatalogOperations` executes `await gumroad.help()` and compares all returned operation names against `mcp.NewCatalog(cmd.NewRootCmd).Operations()`.
2. `TestReadExecutesWithoutConfirmation` executes `await gumroad.products.list()` and asserts one authenticated `GET /products` and an `ok: true` JSON envelope.
3. `TestMutationPlansWithoutConfirmation` executes `await gumroad.products.create({name:"Draft"})`, asserts no HTTP request occurs, and compares the returned plan to `{"executed":false,"requires_confirmation":true,"operation":"products.create",...}`.
4. `TestMutationExecutesOnlyWithExactConfirmation` first calls `confirm: false` and asserts no request, then calls `{confirm:true}` and asserts the expected POST request exactly once.
5. `TestReadRejectsConfirmationOptions` calls `gumroad.products.list({}, {confirm:true})` and asserts the structured `invalid_options` error with no request.

- [ ] **Step 2: Run runtime safety tests to verify RED**

Run: `go test ./internal/cmd/codemode -run 'Test(HelpCoversCatalogOperations|ReadExecutesWithoutConfirmation|MutationPlansWithoutConfirmation|MutationExecutesOnlyWithExactConfirmation|ReadRejectsConfirmationOptions)' -count=1`

Expected: FAIL because `Runtime.Execute` and JavaScript bindings do not exist.

- [ ] **Step 3: Implement one fresh constrained runtime per request**

Implement `Runtime.Execute` using a new `goja.Runtime` for each call. Do not install Go-backed APIs other than a generated `gumroad` object. For each catalog operation, create nested JavaScript objects from the dot-separated operation name and attach a Go function that:

1. accepts zero, one, or two JS arguments;
2. exports the first only as `map[string]any` and JSON-encodes it into the catalog argument shape;
3. validates the second only as a plain object with the single boolean property `confirm`;
4. returns a non-executed plan for an unconfirmed mutation;
5. rejects options for a read;
6. calls `Catalog.Execute` only for a read or confirmed mutation; and
7. parses its JSON response into a JS value before returning it.

Attach `gumroad.help` as a separate function that accepts no arguments or one operation-name string and delegates to `Catalog.Help`. Wrap submitted source as an async IIFE so callers can use `await`; resolve immediate Go return values synchronously. Use `Runtime.Interrupt` from a context/deadline watcher to terminate infinite JavaScript evaluation; discard the runtime after every invocation.

- [ ] **Step 4: Run runtime safety tests to verify GREEN**

Run: `go test ./internal/cmd/codemode -run 'Test(HelpCoversCatalogOperations|ReadExecutesWithoutConfirmation|MutationPlansWithoutConfirmation|MutationExecutesOnlyWithExactConfirmation|ReadRejectsConfirmationOptions)' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the Code Mode API**

Run: `gofmt -w internal/cmd/codemode && go test ./internal/cmd/codemode -count=1`

Commit:
```bash
git add internal/cmd/codemode/runtime.go internal/cmd/codemode/server.go internal/cmd/codemode/codemode_test.go
git commit -m "feat: expose Gumroad operations through Code Mode"
```

## Task 4: Add abuse resistance and explicit output bounds

**Files:**
- Modify: `internal/cmd/codemode/server.go`
- Modify: `internal/cmd/codemode/runtime.go`
- Modify: `internal/cmd/codemode/codemode_test.go`

**Interfaces:**
- Defines named constants `maxCodeBytes`, `executionTimeout`, and `maxResultBytes` in `server.go`.
- Oversized successful output returns `{"ok":true,"truncated":true,"summary":{"observed_bytes":N},"value":null}`.
- Runtime, validation, command, and timeout errors return the common error envelope and are marked MCP tool errors.

- [ ] **Step 1: Write failing boundary tests**

Add tests that:

- submit source longer than `maxCodeBytes` and expect `code_too_large` before runtime execution;
- execute `while (true) {}` and expect `execution_timeout` within a test context deadline;
- call an unknown namespace/method, use `process`, and invoke an operation with a bad flag type, each expecting a structured error and no HTTP request;
- configure a tiny test-only `maxResultBytes`, return a valid large product response, and assert `truncated:true`, `value:null`, and the observed byte count rather than a partial document;
- make the provider return a response body containing a sentinel token-like string and assert every error envelope omits that sentinel.

- [ ] **Step 2: Run the boundary tests to verify RED**

Run: `go test ./internal/cmd/codemode -run 'Test(CodeTooLarge|ExecutionTimeout|RuntimeRejectsUnsupportedGlobals|OversizedResultIsExplicit|ErrorsRedactProviderBody)' -count=1`

Expected: FAIL because the limits and structured error behavior are not implemented.

- [ ] **Step 3: Implement limits and safe errors**

Reject code bytes above `maxCodeBytes` before creating a Goja runtime. Bind execution to the request context plus `executionTimeout`; on expiry call `Runtime.Interrupt` with a private sentinel and translate only that sentinel to `execution_timeout`.

After converting a successful JS value to JSON, count bytes before building MCP content. If it exceeds `maxResultBytes`, replace it with the specified truncation envelope; do not return a prefix. Normalize all handler failures through one error constructor that emits a stable public code/message and never appends raw stderr, provider response bodies, credentials, authorization headers, or URLs.

- [ ] **Step 4: Run boundary and package tests to verify GREEN**

Run: `go test ./internal/cmd/codemode -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the safety boundary**

Run: `gofmt -w internal/cmd/codemode && go test ./internal/cmd/codemode -count=1`

Commit:
```bash
git add internal/cmd/codemode
git commit -m "feat: bound Gumroad Code Mode execution"
```

## Task 5: Document, verify, and switch Hermes to the compact server

**Files:**
- Modify: `skills/gumroad/SKILL.md`
- Modify: `/Users/hermes/work/bin/gumroad-pp-mcp`
- Modify: `~/.hermes/config.yaml`

**Interfaces:**
- `gumroad codemode` is the documented machine entry point.
- The local wrapper executes `/Users/hermes/work/bin/gumroad-cli-pp codemode` without `--dry-run`.
- Hermes config has one enabled `gumroad-pp` server with an include list containing only `gumroad`.

- [ ] **Step 1: Write documentation assertions before editing docs**

Add a test in `internal/cmd/codemode/codemode_test.go` that executes `gumroad codemode --help` and asserts it contains a runnable JavaScript example for a read plus a mutation example with `{confirm: true}`. Add a test that asserts the root command help includes `gumroad codemode`.

- [ ] **Step 2: Run documentation tests to verify RED**

Run: `go test ./internal/cmd/codemode -run 'Test(CodeModeHelp|RootHelpMentionsCodeMode)' -count=1`

Expected: FAIL until explicit help/example text is added.

- [ ] **Step 3: Document and build**

Update `NewCodeModeCmd` help text and `skills/gumroad/SKILL.md` with the `gumroad` API contract: use `gumroad.help()`, reads execute directly, and mutations return plans until `{confirm:true}`. Do not include credentials or merchant data.

Run:
```bash
gofmt -w internal/cmd/codemode internal/cmd/root.go
make build
```

Replace the wrapper content with:
```sh
#!/bin/sh
set -eu
exec /Users/hermes/work/bin/gumroad-cli-pp codemode
```

Build `/Users/hermes/work/bin/gumroad-cli-pp` from the verified current head. Update the existing `gumroad-pp` Hermes server to include only `gumroad`; do not add any credential environment field because the protected CLI config remains the token source.

- [ ] **Step 4: Verify local protocol and live read-only operation**

Run the full local suite:
```bash
PATH=/Users/hermes/work/bin:$PATH make test-cover
PATH=/Users/hermes/work/bin:$PATH make lint
make test-race
go build -o /Users/hermes/work/bin/gumroad-cli-pp ./cmd/gumroad
```

Then use an MCP stdio client to initialize `/Users/hermes/work/bin/gumroad-cli-pp codemode`, assert its tool list is exactly `gumroad`, and execute:
```javascript
await gumroad.products.list()
```

Assert the response is a parsed success envelope with a product list; report only its count. This is read-only and must not be executed with `confirm`.

Read back the exact `mcp_servers.gumroad-pp` stanza from `~/.hermes/config.yaml`, verify it has `tools.include: [gumroad]`, and run `hermes mcp list` to confirm `gumroad-pp` shows one selected tool.

- [ ] **Step 5: Commit source/docs; do not commit host configuration or binaries**

Commit:
```bash
git add go.mod go.sum internal/cmd/mcp internal/cmd/codemode internal/cmd/root.go skills/gumroad/SKILL.md
git commit -m "feat: add compact Gumroad Code Mode MCP"
```

Do not add `/Users/hermes/work/bin/gumroad-cli-pp`, `/Users/hermes/work/bin/gumroad-pp-mcp`, `~/.hermes/config.yaml`, token stores, or live output to Git.

## Plan Self-Review

- **Spec coverage:** Tasks 1–3 preserve complete operation coverage, enforce one-tool Code Mode, reuse the Cobra command boundary, provide `gumroad.help`, and gate writes on exact `confirm:true`. Task 4 provides timeout, safe errors, and explicit bounded output. Task 5 switches and verifies Hermes only after tests and a live read.
- **No placeholders:** The plan contains no incomplete markers or implicit error-handling steps; each task specifies files, interfaces, test commands, expected red state, green state, and commits.
- **Type consistency:** `mcp.Catalog` is the shared source of operation metadata and execution; `codemode.Runtime` depends only on that catalog; `codemode.NewServer` owns the single MCP `gumroad` tool; `NewCodeModeCmd` owns stdio wiring and root registration.
