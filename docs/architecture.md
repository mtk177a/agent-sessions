# Architecture

Use this document when changing `agent-sessions`: it explains the responsibilities, data flow, and constraints that a change must preserve.\
For command syntax and JSON fields, use the [CLI JSON reference](cli-json-contract.md); for accepted provider formats, use the [provider documents](#maintaining-provider-adapters).

## Responsibilities and data flow

`agent-sessions` reads provider-owned interaction records and returns normalized observations.\
The provider retains the raw records; the caller decides what those observations mean and what to retain.

```text
Provider-owned records
  Codex home / Claude Code root / selected ChatGPT export ZIP
      │
      ▼
Configuration resolution → named source instances
      │
      ▼
Provider adapter → discovery, identity, decoding, source evidence
      │
      ▼
CLI core → validation, bounds, pagination, JSON response
      │
      ▼
Caller → interpretation, retained evidence, processing state
```

The implementation separates these responsibilities:

| Component | Responsibility |
| --- | --- |
| [`cmd/agent-sessions`](../cmd/agent-sessions) | Register the production adapters and start the CLI. |
| [`internal/config`](../internal/config) | Load machine-local configuration and resolve provider-level roots. |
| [`internal/provider`](../internal/provider) and its adapters | Discover records and implement provider-specific access, normalization, and verification evidence. |
| [`internal/cli`](../internal/cli) | Dispatch operations and validate, bound, paginate, and encode results. |
| [`internal/contract`](../internal/contract) | Define common response types, source references, content bounds, and version hashing. |
| [`internal/safeio`](../internal/safeio) | Open and read bounded files while enforcing source containment and replacement checks. |

Provider-specific layouts, row types, and correlation identifiers stay inside their adapters.\
They are version-sensitive decoder inputs, not extensions to the stable public schema.

## Identity: provider, source instance, source

These three concepts allow multiple independent stores for one provider without confusing their records:

| Concept | Meaning | Example |
| --- | --- | --- |
| Provider | The adapter used to access records. | `codex`, `claude`, or `chatgpt` |
| Source instance | A separately named store or access boundary belonging to one provider. | `codex-one` and `codex-two` for two Codex homes |
| Source | One independently addressable provider-owned interaction record within that instance. | A Codex thread, Claude Code session, or ChatGPT conversation |

A provider name identifies an access adapter, not a model vendor in the abstract.\
A source instance has a stable logical ID and a machine-local locator, such as a provider home or export path.\
A source is identified by its provider, source instance, and provider-native source ID.

Paths are locators, not identities.\
Moving a provider record without changing its provider identity does not itself create a new source.\
The same provider-native ID in two independently named instances does not make them the same source.\
Renaming an instance is logically significant to callers that retain source references.

The CLI uses deterministic `source_ref` values rather than embedding native IDs or local paths in command arguments.\
The [logical identity specification](cli-json-contract.md#logical-identity) defines their encoding and required fields.

### Why configuration uses provider-level roots

User configuration identifies a Codex home, Claude Code configuration root, or exact ChatGPT export ZIP.\
The adapter owns the internal layout below that boundary, so a provider's directory changes do not become ordinary user configuration changes.

Ordinary single-store Codex and Claude Code setups can use provider environment variables or defaults.\
Nonstandard homes, mounted data, and multiple stores use explicit roots or named instances.\
ChatGPT exports are always explicitly selected snapshots.

Machine-specific paths belong in machine-local configuration.\
The [configuration reference](cli-json-contract.md#configuration) defines selection, precedence, and failure handling.

## Observations: events, time, and content versions

An event is an ordered normalized observation from a source: a message, tool call, tool result, or error.\
Sources expose explicit relationships; source and event metadata may retain additional provider information when materially available.\
Normalization must preserve meaningful provider differences rather than inventing equivalence.

Adapters establish tool outcomes and relationships only from verified, explicit provider fields.\
Output text such as `PASS`, matching text, or adjacent records cannot establish success or a call/result relationship.\
Readable bodies can remain available even when their outcome or correlation is unknown; the uncertainty is reported as an omission.

`last_interaction_at` describes the latest established message or tool interaction in the logical source.\
It is distinct from a file's modification time or provider management activity.\
The [source-time specification](cli-json-contract.md#source-interaction-time) and provider documents define the accepted observations and missing-time behavior.

### Identity and version answer different questions

A resumed session can retain its identity while its content changes.

- **Version hint**: an optimization signal for deciding whether a source may have changed; it is not durable cryptographic evidence.
- **Verified version**: Comes from the stronger source-content read performed by `verify` and can be retained as provenance.

A caller can list sources, compare hints with its own state, verify possible changes, and read normalized events.\
Reviewed versions, findings, proposals, import decisions, and retained evidence belong to that caller.\
Two callers can independently review the same version; the access layer neither infers nor reconciles their states.

Reference formats, hint algorithms, verification hashing, and the public response schema have independent versions.\
A schema change alone must not change an otherwise identical source reference, hint, or verified version.\
Exact reference and verification encodings are in the CLI reference; provider documents define their hint algorithms.

## Read-only access and resource behavior

Provider records remain the raw source of truth.\
Inspection must not modify provider history, provider application metadata, caller state, or persistent `agent-sessions` state.\
The host file system may update access-time metadata on a file read; that side effect is outside the semantic no-write guarantee.

Prefer documented provider interfaces when they offer sufficient discovery, fidelity, bounded access, and a non-mutating read path.\
Official status alone is not enough: an interface that repairs metadata, migrates data, or synchronizes persistently may require a safer supported access path.\
Direct file parsing is acceptable when needed to preserve the stronger read-only guarantee.

File-backed adapters open only bounded regular files.\
Directory-backed adapters recheck opened file identity, resolved containment, and root identity; exact-file adapters recheck the selected file and content snapshot.\
Special files and path replacement outside the configured boundary are rejected.

### On-demand work, no persistent application state

Given the same source data, configuration, and executable version, results must not depend on previous executions.\
There is no registry bootstrap, authoritative session database, transcript mirror, persistent index, or background event spool.

No daemon, watcher, timer, periodic synchronization, or lifecycle-hook ingestion is required.\
When no command runs, CPU use, memory use, and disk I/O from the tool are zero.\
Read commands cause no persistent `agent-sessions` writes.

Read only what the requested observation requires, using cheaper metadata to narrow sources when possible.\
Identity discovery alone may read a header, but establishing interaction time requires more content: Codex `list` reads bounded effective rollout history, including inherited prefixes, and Claude Code `list` reads bounded main transcripts.\
The accepted formats and input limits are maintained in each provider document.

### Conditions for a future cache

SSD write endurance is a design concern.\
Introduce a cache only when realistic measurements demonstrate a material discovery or parsing cost, and review [ADR 0001](decisions/0001-use-stateless-read-only-source-access.md) first.

Any cache must be disposable and non-authoritative: removal may affect performance only.\
It must not become caller processing state, require unnecessary persistent writes for unchanged reads, or introduce idle disk writes.\
Measure write amplification and any new persistent I/O on the affected operating systems with realistic source volumes.\
Do not introduce periodic whole-history scans merely to improve interactive latency.

## Maintaining provider adapters

Use the provider document for the adapter being changed:

- [Codex](codex.md): home selection, inherited and sliced history, accepted versions, normalization, and input limits.
- [Claude Code](claude.md): configuration root, main transcripts, subagent limitations, accepted versions, and normalization.
- [ChatGPT Data Export](chatgpt.md): explicit ZIP selection, active branches, snapshot identity, and archive bounds.

### Separate discovery from decoding

Documented roots, transcript locations, or export acquisition establish the discovery boundary.\
Internal JSONL rows and conversation graphs remain separately version-sensitive compatibility inputs, as described in [ADR 0003](decisions/0003-separate-provider-discovery-from-version-sensitive-decoding.md).

Verify current provider documentation or implementation when changing that boundary.\
Keep compatibility evidence, decoding rules, and independently constructed synthetic tests with the owning adapter.\
Do not silently accept an unknown format as complete or expose provider internals as stable common fields.

### Preserve incomplete observations and input safety

Successful parsing does not establish complete observation.\
Keep useful but incomplete results, recognized unsupported observations, and failures distinct.\
Report omitted, truncated, unsupported, or unknown material values; never hide them behind `complete`.\
The [CLI completeness rules](cli-json-contract.md#completeness-and-omissions) define statuses, pagination, and aggregation across providers.

Historical content is untrusted data.\
Do not execute recorded commands, follow historical URLs or tool requests, or grant authority to instructions found in records.\
Recorded content is returned without automatic redaction; callers decide what to analyze, retain, or share.\
Structured diagnostics must not unnecessarily copy source data or underlying errors.

Bound malformed, nested, compressed, oversized, and other adversarial inputs.\
Use only synthetic tests and fixtures, with obviously fictional credential-like values.\
Do not create committed fixtures by redacting real private sessions, and do not commit private session contents or machine-specific information.

Run the applicable [development validation commands](../README.md#development-validation) after implementation changes.

## Architecture decisions

The accepted ADRs record why the current responsibilities and constraints were chosen.\
Change or supersede the affected ADR if an architectural decision changes, then keep documentation, implementation, and tests consistent.

| Decision | Question it answers |
| --- | --- |
| [0001: Stateless, read-only source access](decisions/0001-use-stateless-read-only-source-access.md) | Why avoid a shared registry, transcript archive, and background processing? |
| [0002: Named source instances](decisions/0002-model-provider-sources-as-named-instances.md) | Why separate logical identity from storage paths? |
| [0003: Discovery and version-sensitive decoding](decisions/0003-separate-provider-discovery-from-version-sensitive-decoding.md) | Which provider facts can discovery rely on, and which formats need compatibility checks? |
| [0004: Bounded content without automatic redaction](decisions/0004-return-bounded-recorded-content-without-redaction.md) | Why return recorded inputs and results, and what exposure does that require callers to handle? |

Domain-specific interpretation, personal memory, retrospective workflows, a general chat client or web UI, cloud storage or synchronization, continuous telemetry, and full-text or vector indexing are outside the access layer's responsibilities.\
A separate caller can build on the read interface when such a need exists.
