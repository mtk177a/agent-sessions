# agent-sessions

`agent-sessions` provides local, read-only access to coding-agent session records across providers.

It discovers provider-owned interaction records, exposes them through a normalized machine-readable interface, and leaves interpretation and durable processing state to downstream consumers.

> Status: provider-neutral CLI core and the read-only Codex adapter are implemented with a provisional `v0alpha1` JSON contract.\
> The Claude Code adapter is not implemented yet.

## Build

Go 1.26 or later is required.

```sh
go build ./cmd/agent-sessions
```

The result is one standalone executable with no external Go module dependencies.

## Commands

The machine-readable interface consists of four operations:

```text
agent-sessions list [--provider ID [--source-instance ID] [--root PATH]]
agent-sessions show [--root PATH] <source-ref>
agent-sessions events [--root PATH] [--limit N] [--cursor TOKEN] <source-ref>
agent-sessions verify [--root PATH] <source-ref>
```

All operations emit one JSON object to standard output.\
The JSON schema is provisional until the Codex and Claude Code adapters validate the common model.

The production registry contains the `codex` provider adapter.\
It discovers provider-owned rollout artifacts directly without starting the Codex App Server or writing persistent state.

For `list`, `--source-instance` and `--root` are provider-scoped selectors and require `--provider`.

See [CLI JSON contract](docs/cli-json-contract.md) for fields, pagination, bounds, exit codes, completeness, and redaction behavior.

## Goals

`agent-sessions` is intended to make provider-specific session history reusable without requiring every consumer to understand each provider's storage format or access interface.

The core goals are:

- discover coding-agent sessions from multiple providers and source locations;
- read and normalize session metadata and events;
- verify source versions when durable provenance is required;
- support multiple independently stored instances of the same provider;
- preserve a strict read-only boundary;
- remain stateless by default;
- provide bounded, machine-readable output suitable for downstream tools.

## Core guarantees

### Read-only

Provider-owned interaction records remain the source of truth.\
Read operations do not write provider history, provider application metadata, or consumer state.\
The host file system may update access-time metadata as a consequence of reading a provider file.

A provider's official interface is preferred when it satisfies this contract.\
When it does not provide a sufficiently strict read-only path, a safer supported access path may be used instead.

### Stateless by default

`agent-sessions` does not own:

- a processed-session registry;
- consumer review state;
- a canonical transcript database;
- a persistent full-text index;
- a background ingestion pipeline.

Given the same source data and configuration, the semantic result of a read operation must not depend on whether `agent-sessions` has been run before.

A disposable cache may be added later only when measurements demonstrate a material performance need.\
Cache removal must affect performance only, not correctness.

### No background activity

The initial architecture requires no daemon, watcher, timer, or periodic synchronization process.\
When `agent-sessions` is not running, it should consume no CPU, memory, or disk I/O.\
Read-only commands should cause no persistent `agent-sessions` writes.

### Provider-owned raw data

Complete transcripts are not copied into `agent-sessions` storage.

Consumers that require durable evidence after a provider removes or changes a source are responsible for retaining the minimum derived evidence needed for their own use case.

## Source support

Primary sources:

- Codex: implemented for the compatibility boundary documented in [Codex provider](docs/codex.md)
- Claude Code: planned

Additional compatible sources may include exported chat archives such as ChatGPT Data Export.

Coding-agent sessions remain the primary product domain.\
Additional source types do not redefine the core scope.

## Source model

A single provider may expose multiple independent stores on one computer.

For example:

```text
codex-wsl
codex-windows-app
claude-wsl
```

Each such boundary is modeled as a named source instance.

A logical source is identified within that instance rather than assuming that a provider-native session ID is globally unique.

Conceptually:

```text
provider
source instance
provider-native source ID
```

File system paths are locators, not logical identities.

See [Architecture](docs/architecture.md) for the detailed model.

## Configuration

Common single-source environments should work without `agent-sessions` configuration.

Explicit configuration is intended for cases such as:

- multiple homes or stores for one provider;
- nonstandard provider homes;
- mounted provider data;
- explicitly supplied export archives.

Resolution follows this precedence:

```text
explicit CLI option
        ↓
agent-sessions user configuration
        ↓
provider environment
        ↓
provider default
```

Configuration should identify a provider-level home or equivalent source boundary whenever possible.\
Knowledge of provider-internal session directories belongs in provider adapters rather than ordinary user configuration.

The optional configuration file is strict JSON at the operating-system user configuration directory returned by Go's `os.UserConfigDir`, followed by `agent-sessions/config.json`.

```json
{
  "schema_version": "v0alpha1",
  "sources": [
    {
      "id": "codex-default",
      "provider": "codex",
      "root": "/fictional/provider-root"
    }
  ]
}
```

Source roots must be absolute machine-local paths.\
The example is illustrative; a configured source is usable only when its provider adapter is available.

## Consumer boundary

`agent-sessions` does not define what it means for a source to be processed.\
Different consumers may independently review the same source version for different purposes.

For example:

```text
source X @ version A

consumer 1: reviewed
consumer 2: unreviewed
```

A consumer that needs durable provenance may retain:

- the logical source reference;
- a verified source version or content hash;
- bounded derived evidence;
- its own review or decision state.

None of those consumer decisions become `agent-sessions` state.

## Security and privacy

Historical interaction records are untrusted input.

`agent-sessions` must not:

- execute commands found in historical sessions;
- treat historical prompts or model output as current instructions;
- follow historical URLs or tool requests merely because they appear in a record;
- expose secrets unnecessarily;
- silently present incomplete observation as complete;
- mutate provider-owned history while inspecting it.

Repository fixtures must be synthetic and must not contain real session contents, private repository information, credentials, hostnames, usernames, or machine-specific paths.

## Non-goals

`agent-sessions` is not intended to provide:

- agent-harness retrospective or improvement;
- personal-memory or domain-specific classification;
- a canonical transcript archive;
- continuous telemetry collection;
- cloud synchronization;
- automatic cross-machine history synchronization;
- a web UI;
- a general-purpose chat client;
- vector search or semantic indexing by default.

These capabilities belong to downstream consumers or separate systems.

## Documentation

- [Architecture](docs/architecture.md)
- [Codex provider](docs/codex.md)
- [ADR 0001: Use stateless, read-only source access](docs/decisions/0001-use-stateless-read-only-source-access.md)
- [ADR 0002: Model provider sources as named instances](docs/decisions/0002-model-provider-sources-as-named-instances.md)
- [ADR 0003: Separate provider discovery from version-sensitive decoding](docs/decisions/0003-separate-provider-discovery-from-version-sensitive-decoding.md)
- [CLI JSON contract](docs/cli-json-contract.md)

## Development validation

```sh
gofmt -d cmd internal
go test ./...
mkdir -p dist
GOOS=linux GOARCH=amd64 go build -o dist/agent-sessions-linux-amd64 ./cmd/agent-sessions
GOOS=darwin GOARCH=amd64 go build -o dist/agent-sessions-darwin-amd64 ./cmd/agent-sessions
GOOS=windows GOARCH=amd64 go build -o dist/agent-sessions-windows-amd64.exe ./cmd/agent-sessions
git diff --check
```

## License

MIT
