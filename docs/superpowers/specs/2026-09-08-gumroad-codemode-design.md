# Gumroad Code Mode Design

## Goal

Expose every public Gumroad CLI operation through one compact, Printing Press-owned MCP tool without presenting the upstream server's 100-operation tool list to an agent.

## Scope

This project adds a Code Mode transport alongside the existing `gumroad mcp` command. The existing CLI command surface and generated 100-tool MCP server remain compatible and unchanged. Hermes will be reconfigured to expose only the Code Mode tool once the new server is verified.

The adapter covers every operation currently reached by the upstream MCP command tree, excluding the existing excluded command groups: `admin`, `auth`, `completion`, `help`, `mcp`, and `skill`.

## Architecture

### One MCP tool

The new server advertises exactly one tool, named `gumroad`. Its input is a JavaScript program. The tool evaluates the program in a constrained Code Mode runtime that provides a single `gumroad` object and no shell, filesystem, network, process, or credential APIs.

The runtime exposes an async namespaced API derived from the same Cobra command tree used by the CLI and the existing generated MCP server:

```javascript
await gumroad.products.list()
await gumroad.products.update({ args: ["product-id"], name: "Updated name" }, { confirm: true })
await gumroad.sales.refund({ args: ["sale-id"] }, { confirm: true })
await gumroad.help("products.update")
```

Each callable maps to one command path. Method names follow the existing generated MCP convention: hyphens become underscores, and command paths become nested object names. Tool discovery and schemas are available at runtime through `gumroad.help()` rather than being expanded into MCP tool definitions.

### Command execution boundary

The adapter reuses the existing command-tree walk, command flag schema construction, argument decoding, authentication check, stdin isolation, and in-process Cobra execution. It never launches a shell or accepts an arbitrary command string. A Code Mode call can select only a discovered command path and can pass only positional arguments and flags accepted by that command.

CLI credentials continue to be resolved by the upstream configuration layer at execution time. Tokens are not part of Code Mode input, output, logs, schemas, or Hermes configuration.

## Safety

### Operation classification

A command is read-only only when the existing generated MCP metadata identifies it as read-only. Every other command is a mutation. This deliberately preserves the upstream server's conservative classification; no method-name heuristic can silently downgrade an operation.

### Confirmation

Read-only operations execute immediately. A mutation requires its second options argument to contain exactly `confirm: true` before any command is executed.

Without confirmation, the runtime returns a structured, non-executed plan:

```json
{
  "executed": false,
  "requires_confirmation": true,
  "operation": "products.update",
  "arguments": {"args": ["product-id"], "name": "Updated name"}
}
```

The plan does not fetch or mutate provider state. With `confirm: true`, the adapter invokes the underlying command with its existing MCP safety flags (`--json`, `--no-input`, `--quiet`, and `--yes` when supported) and returns the parsed JSON or a safe execution error.

`confirm: false`, omitted confirmation, malformed confirmation objects, and `confirm` provided to read-only calls do not execute a mutation. Confirmation is an operation-level contract and does not make a read-only command mutable.

### Error and output handling

Validation failures, authentication failures, command errors, JavaScript errors, timeouts, and output-limit violations are returned as structured errors. Error output must not include tokens, request authorization headers, full authenticated URLs, or raw provider response bodies.

The runtime applies a fixed per-call timeout and a fixed serialized-result byte limit. Outputs above the limit return a valid JSON envelope with `truncated: true`, the observed byte count, and no partial JSON value. This makes incomplete output explicit.

## Runtime API

`gumroad.help()` with no argument returns the available namespaces and operation names. `gumroad.help("namespace.operation")` returns that operation's positional-argument contract, accepted flags with JSON types, read-only/mutation classification, and confirmation requirement.

Every operation accepts:

1. an optional argument object matching its original generated MCP schema: `args` is a string array and all other keys are supported Cobra flags; and
2. an optional options object, currently limited to `confirm: true` for mutations.

Unknown operation names, malformed argument objects, unknown flags, wrong JSON flag types, positional arguments that resemble flags, and unsupported runtime globals are rejected before command execution.

## Testing

Tests use the in-memory MCP transport and a deterministic JavaScript runtime. They prove:

- the Code Mode server enumerates exactly one tool;
- runtime help covers every public operation present in the existing generated MCP server;
- a representative read executes without confirmation;
- a representative mutation returns a plan and does not issue an HTTP request without `confirm: true`;
- the same mutation executes only with `confirm: true`;
- a malformed or false confirmation cannot execute a mutation;
- generated argument schemas and type validation remain consistent with the existing command adapter;
- invalid JavaScript, unknown operations, unsupported runtime APIs, authentication failures, command errors, timeouts, and oversized results return safe structured errors;
- no test fixture or output contains credentials.

## Hermes integration

After local and live read-only verification, Hermes replaces the current nine-tool `gumroad-pp` filter with this server's single `gumroad` tool. The protected upstream CLI token store remains the authentication source. The legacy generated MCP entry is not exposed through Hermes.

## Non-goals

- Changing, removing, or broadening upstream CLI operations.
- Changing the behavior of `gumroad mcp`.
- Implementing a shell passthrough, arbitrary HTTP client, filesystem API, or process API.
- Automatically confirming provider mutations.
- Releasing, publishing, or submitting an upstream pull request.
