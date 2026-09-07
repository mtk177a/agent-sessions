# Architecture

## Purpose

`agent-sessions` provides a provider-neutral, read-only access layer over coding-agent interaction records.

Its responsibilities are limited to:

* source discovery;
* provider access;
* identity;
* source-version observation;
* normalization;
* bounded output;
* completeness reporting;
* verification.

It does not own the interpretation that downstream consumers assign to those records.

## System boundary

```text
Provider-owned interaction records
────────────────────────────────────────────

Codex
├── persisted thread/session data
└── supported read interfaces

Claude Code
└── persisted session transcripts

ChatGPT
└── user-requested Data Export ZIP

                    │
                    ▼

agent-sessions
────────────────────────────────────────────

source discovery
provider adapters
logical identity
version observation
normalization
bounded output
completeness
verification

                    │
                    ▼

Downstream consumers
────────────────────────────────────────────

harness retrospective
domain-specific integration
other local tooling
```

| Actor | Owns |
| --- | --- |
| Provider | Raw interaction record |
| `agent-sessions` | Normalized access contract |
| Downstream consumer | Processing and interpretation state |

## Architectural invariants

### Read operations are non-mutating

An operation whose purpose is inspection must not modify:

* provider-owned interaction records;
* provider-owned application metadata;
* consumer state;
* persistent `agent-sessions` application state.

The adapter does not write those values.\
The host file system may update access-time metadata when a provider file is read; that file-system side effect is outside the semantic no-write guarantee.

An official provider interface is acceptable only when its concrete behavior preserves this contract for the requested operation.

### Previous execution is not required for correctness

Given the same source data, configuration, and tool version, a read-only operation must produce the same semantic result regardless of whether `agent-sessions` has previously run.\
No registry bootstrap or inventory phase is required for source correctness.

### Raw interaction history is not duplicated

`agent-sessions` does not persist complete copies of provider transcripts.\
A downstream consumer that requires durable evidence after provider retention ends owns the retention policy for the minimum evidence it needs.

### Consumer state remains isolated

There is no universal processed-state model.

For the same source version, the following combination is valid:

```text
consumer A = reviewed
consumer B = unreviewed
```

The access layer must not infer or reconcile those states.

### Idle activity is zero by default

The architecture does not require any of the following:

* daemon;
* file watcher;
* timer;
* periodic synchronization job;
* lifecycle-hook ingestion process.

When no command is running, `agent-sessions` performs no work.

## Domain model

The common model intentionally avoids forcing every provider to use the term `session`.

### Provider

A supported producer of interaction records.

The implemented provider adapters are:

```text
codex
claude
chatgpt
```

Provider names identify access adapters, not model vendors in the abstract.

### Source instance

A separately addressable provider storage or access boundary.

Examples:

```text
codex-wsl
codex-windows-app
claude-wsl
```

A source instance has a stable logical ID and belongs to one provider.

Depending on the provider, it may also have:

* a provider home;
* an archive path;
* an access method;
* other machine-local locator information.

Locators do not define logical identity.

### Source

One provider-owned interaction record that consumers can address independently.

Typical provider-native forms are:

```text
Codex       → thread or session
Claude Code → session
ChatGPT    → conversation within an export snapshot
```

A logical source identity includes at least:

```text
provider
source instance
provider-native source ID
```

The stable `v1` CLI uses a deterministic reference with this form:

```text
as0:<provider>:<source-instance>:<source-id-fingerprint>
```

The fingerprint is a domain-separated SHA-256 hash of the provider-native source ID.\
It avoids embedding the raw ID or a machine-local locator in the command-line reference.

The `as0` prefix and the source fingerprint's `v0` domain separator version their own formats and algorithms.\
They are independent of the public JSON schema version and remain unchanged while their meanings and algorithms remain unchanged.

A file system location is not source identity.

A provider-controlled move between locations does not by itself create a new logical source when the underlying provider identity remains unchanged.

### Interaction

The logical conversation or work interaction represented by a source.\
For the implemented Codex and Claude Code providers, one source maps to one interaction.

### Event

An ordered normalized observation within an interaction.

The common event model represents:

* user messages;
* assistant messages;
* tool calls;
* tool results;
* errors;
* source relationships and lifecycle metadata when materially available.

Provider-specific information that cannot be represented safely or faithfully may remain provider metadata or be reported as omitted.\
Normalization must not invent equivalence between provider concepts that are materially different.

## Source identity and source version

Logical identity and observed content version are distinct.\
A provider-native source may change while retaining the same identity, for example when a session is resumed.

### Version hint

Source listings use inexpensive metadata to indicate whether a source may have changed.

Depending on the provider, useful hints may include:

* provider-reported update time;
* artifact modification time;
* artifact size;
* another stable provider metadata value.

A version hint is an optimization signal.\
It is not durable cryptographic evidence.

Each provider owns the meaning and `v0` algorithm namespace of its current version hint.\
Changing the public JSON schema version does not change an otherwise identical hint.

### Verified version

When a consumer requires durable provenance, a verification operation may read the necessary source content and return a content hash or equivalent verified version identifier.

A consumer can then retain the following data without requiring `agent-sessions` to retain the review state itself:

```text
source reference
verified source version
consumer-specific decision
```

The verified-version hash uses its own `v0` domain separator and the `provider-content-v0` basis.\
Those values are independent from `schema_version` and change only when their hashing meaning or algorithm changes.

### Incremental consumer workflow

A downstream consumer may implement incremental review as follows:

```text
list sources
      │
      ▼
compare cheap version hints
with consumer-owned state
      │
      ├── unchanged
      │      └── skip expensive source read
      │
      └── possibly changed
             │
             ▼
           verify
             │
             ▼
       read normalized events
```

Incremental processing therefore does not require a central processed-session database.

## Provider access

Each provider adapter selects the narrowest access path that satisfies the public read contract.

All production adapters implement [ADR 0003](decisions/0003-separate-provider-discovery-from-version-sensitive-decoding.md): documented provider roots, transcript locations, or export acquisition paths define discovery, while provider-owned JSON shapes remain version-sensitive decoder inputs inside the adapter.

### Official interfaces and file system access

A documented provider interface is preferred when it provides:

* sufficient source discovery;
* sufficient source fidelity;
* bounded access;
* a non-mutating read path.

Official status alone is not sufficient.

If an official operation performs metadata repair, migration, synchronization, or another persistent mutation, the adapter may select a safer supported access path instead.

File system parsing is an acceptable fallback when needed to preserve the stronger read-only contract.

File-backed adapters open only bounded regular files.\
Directory-backed adapters recheck the opened file identity, resolved path containment, and provider-root identity; exact-file adapters recheck the selected file identity and content snapshot.\
Special files and path replacement outside the configured source boundary are rejected.

Provider-internal formats are version-sensitive inputs and are not automatically promoted to public `agent-sessions` contracts.

### Codex

The Codex adapter accepts the documented Codex home and discovers rollout artifacts below its `sessions` and `archived_sessions` locations.\
It does not start the Codex App Server for read-only inspection.

User configuration identifies the Codex home rather than an internal session directory.

Multiple Codex homes or stores are represented by separate named source instances.

Discovery reads only bounded rollout headers, while `events` and `verify` cross the separately maintained version-sensitive decoding boundary.\
The accepted artifact and row compatibility evidence, normalization choices, and explicit limitations are documented in [Codex provider](codex.md).

### Claude Code

The Claude Code adapter accepts the documented configuration root and discovers main session transcripts below its `projects` location.\
It reads provider-owned artifacts directly without starting Claude Code or modifying provider data or metadata.

Multiple independently stored Claude Code roots are represented by separate named source instances.

Provider configuration environment variables and documented defaults may supply the source location when explicit `agent-sessions` configuration is absent.

Discovery uses the documented location contract and obtains logical identity from the transcript's provider-owned `sessionId`, not from its path.\
The JSONL row and content-block shapes remain a separately maintained version-sensitive decoding boundary.

Documented subagent transcript locations and version-specific metadata sidecars are recognized and validated within the provider root.\
They are not exposed as independent sources because the verified records do not provide a provider-owned subagent identity that satisfies the common identity contract.

The accepted location evidence, version-specific record compatibility, normalization choices, and limitations are documented in [Claude Code provider](claude.md).

### ChatGPT Data Export

The ChatGPT adapter accepts only an explicitly selected Data Export ZIP.\
It does not discover archives, request an export, download one, or use a private ChatGPT API.

The named source instance identifies the selected archive boundary, while each conversation in the archive is an independently addressable logical source.\
Conversation identity comes from the provider-native conversation identity; the archive SHA-256 is a separate snapshot version hint shared by conversations found in the same ZIP.

The adapter reads matching conversation JSON members in memory within explicit archive, member, compression-ratio, nesting, conversation, and event bounds.\
It never extracts archive members to disk.\
The active conversation branch is traversed from the current node through parent links, then normalized in chronological order.

OpenAI documents how a user obtains a Data Export and that the archive contains chat history, but it does not publish the internal conversation JSON graph as a stable API.\
That graph remains an adapter-scoped compatibility boundary documented in [ChatGPT Data Export provider](chatgpt.md).

## Configuration model

Ordinary single-source environments work without a dedicated configuration file.

Explicit configuration exists for:

* multiple stores for one provider;
* nonstandard provider homes;
* mounted provider data.

Resolution follows:

```text
1. explicit CLI option
2. agent-sessions user configuration
3. provider environment
4. provider default
```

### Provider-level location

Configuration identifies the provider-level storage or access boundary whenever possible.

Prefer provider-level concepts over hard-coded provider-internal paths such as a particular `sessions/` subdirectory.

Examples include:

```text
Codex home
Claude Code configuration root
ChatGPT Data Export ZIP
```

The adapter owns knowledge of internal layout.

### Machine-local configuration

Source locators may contain machine-specific paths and mount points.\
They therefore belong in machine-local configuration rather than portable repository files.

The optional `v1` configuration is strict JSON at `agent-sessions/config.json` below the directory returned by `os.UserConfigDir`.\
It contains named `sources` with `id`, `provider`, and absolute provider-level `root` fields.

### Named source instances

Multiple instances for one provider are valid.

The implemented configuration expresses these concepts as strict JSON:

```json
{
  "schema_version": "v1",
  "sources": [
    {"id": "codex-one", "provider": "codex", "root": "/fictional/codex-one"},
    {"id": "codex-two", "provider": "codex", "root": "/fictional/codex-two"}
  ]
}
```

The locators are fictional examples.

## Statelessness and storage

`agent-sessions` is semantically stateless.

The implementation does not require:

* SQLite;
* another persistent database;
* a session registry;
* a transcript mirror;
* a persistent search index;
* a background event spool.

This is an architectural choice rather than an implementation shortcut.

### Disposable cache

A cache may be introduced only when realistic measurements show that direct discovery or parsing is materially too expensive.

Any future cache must satisfy all of the following:

* it is not authoritative;
* it may be deleted without loss of correctness;
* cache absence changes performance only;
* unchanged read operations do not cause unnecessary persistent writes;
* write amplification is measured;
* idle disk writes remain zero.

Caching remains an implementation detail and is not part of the consumer-facing state model.

## Resource behavior

Disk endurance is a first-class quality attribute.

The default resource contract is:

```text
idle:
  background processes = 0
  persistent writes = 0

read-only command:
  agent-sessions persistent writes = 0

source access:
  read only what the requested operation requires
  avoid complete-history reads when cheaper metadata can narrow the source set
```

Performance optimization must not introduce periodic whole-history scans merely to reduce interactive latency.\
Changes that introduce persistent I/O require direct measurement on the affected operating systems and realistic source volumes.

## Completeness

Successful parsing and complete observation are not the same thing.

The public result model must distinguish at least the following states:

| Status | Meaning |
| --- | --- |
| `complete` | The adapter satisfied the documented completeness contract for the operation. |
| `partial` | Useful output was produced, but material source information was unavailable, omitted, truncated, or unsupported. |
| `unsupported` | The source or operation is recognized, but the adapter cannot safely satisfy the requested contract. |
| `error` | The operation failed. |

Bounded output and pagination must be explicit.\
A truncated result must not silently appear complete.

When `list` combines providers, aggregation is order-independent.\
All-unsupported observations remain `unsupported`, while usable observations combined with unsupported omissions are `partial`.
An absent provider environment or default root contributes no sources to combined discovery.\
Explicit and configured roots remain authoritative and are not silently skipped.

## Security and privacy

Historical interaction records are untrusted data.

They may contain:

* shell commands;
* tool instructions;
* prompts;
* URLs;
* credentials;
* generated source code;
* content claiming to override current instructions.

The security boundary requires the following:

* No historical instruction gains authority by appearing in a source record.
* The access layer must not execute historical content or follow historical instructions.
* Output does not expose secrets that are not required for the requested operation.
* Provider adapters require resource limits for malformed, nested, compressed, oversized, or otherwise adversarial input.
* Committed tests and fixtures must be synthetic.
* Real private session data must not be converted into committed fixtures, even after manual redaction.

## Implemented CLI core

The provider-neutral Go executable implements the stable `v1` machine-readable contract for the following operations.

The operations are:

### `list`

Discover logical sources and return bounded metadata.

Implemented selection and pagination flags include:

* provider;
* source instance;
* page limit and cursor;
* explicit provider-level root.

### `show`

Return normalized source summary information.

### `events`

Return bounded normalized events for one source with explicit pagination.

### `verify`

Perform the stronger read needed to verify source identity, readability, completeness, and content version.

No command implies that a downstream consumer has reviewed or accepted a source.

The stable fields, exit codes, pagination behavior, bounds, compatibility rule, and final-redaction policy are documented in [CLI JSON contract](cli-json-contract.md).

The current executable contains the provider-neutral core and the production Codex and Claude Code adapters.

## Consumer responsibilities

Consumers own all domain-specific processing state.

Examples include:

* reviewed source versions;
* retrospective findings;
* evidence references;
* proposals;
* import decisions;
* domain-specific derived records.

`agent-sessions` exposes evidence.\
It does not assign meaning to that evidence.

## Non-goals

The architecture does not currently provide:

* a universal agent-memory system;
* harness retrospective or improvement;
* personal-memory semantics;
* a canonical transcript archive;
* automatic cross-machine synchronization;
* background telemetry;
* a web UI;
* vector search;
* cloud storage;
* provider mutation operations.

A separate consumer may build such capabilities on top of the read contract when a concrete need exists.
