# Codex provider

Use this document to read saved session records with the `codex` adapter and understand its output.\
The adapter reads saved rollouts without starting the Codex App Server.

For common JSON response fields, see the [CLI reference](cli-json-contract.md).

## Usage procedure

### 1. Select the record location

Select the Codex home.\
Do not select its `sessions` or `archived_sessions` subdirectory.

The tool selects the location in this order:

1. Explicit `--root`.
2. Configured source instance.
3. `$CODEX_HOME`.
4. Documented default, `~/.codex`.

Choose one of the following location-selection methods.

#### Use provider settings

Codex documents `$CODEX_HOME` as its data directory and uses `~/.codex` when that variable is unset.

Without an explicit root or configured instance, use `$CODEX_HOME`.\
If that variable is unset, use `~/.codex`.

In this case, list sessions without a root option:

```sh
agent-sessions list --provider codex
```

#### Use command options

Use `--root` to select the location and list sessions:

```sh
agent-sessions list --provider codex --root /fictional/codex-home
```

#### Use a configuration file

Add a named source instance in the [configuration file](cli-json-contract.md#configuration).\
For example:

```json
{
  "sources": [
    {
      "id": "codex-records",
      "provider": "codex",
      "root": "/fictional/codex-home"
    }
  ]
}
```

Then list sessions from the configured instance:

```sh
agent-sessions list --provider codex --source-instance codex-records
```

### 2. Read a session

Copy `identity.source_ref` for the session from the `list` result into the placeholder below.\
If you used `--root`, pass the same root to this and later source operations:

```sh
agent-sessions events --root /fictional/codex-home '<source_ref from list>'
```

If you use provider settings or a configured instance, omit `--root` from this command.

## Supported formats

Event decoding and format verification accept the verified mode and structure combinations for the 32 observed version labels from `0.92.0` through `0.155.0-alpha.9.2`.

This is an explicit set of labels and history modes, not every version in that interval.\
See the [compatibility table in the adapter](../internal/provider/codex/compatibility.go) for exact accepted combinations.

The operations have these compatibility boundaries:

| Operation | Compatibility boundary |
| --- | --- |
| `events` and `verify` | The explicit version-label and history-mode combinations above. |
| Interaction time in `list` and `show` | Additionally, syntactically valid versions at or after `0.155.0` when all included rows retain known structures; there is no version ceiling. |

Forward-compatible interaction-time reading does not extend verified `events` and `verify` support.\
Unknown rows or unusable timestamps prevent an established source time.

**Compressed rollouts**

Compressed `.jsonl.zst` rollouts are recognized but not decoded because the executable has no external compression dependency.

## Reading the output

### Returned data

The information in `data` depends on the operation.

#### Source information

`list` and `show` return source information for sessions.\
They include `last_interaction_at` when the latest interaction time can be established.

#### Events

`events` returns supported messages, tool calls, tool results, and provider errors.

Each entire input or result follows the common 64 KiB bound; truncation and omissions reduce completeness.

#### Verified version

`verify` returns a verified version of the selected logical history.

### Operation status

#### Status (`status`)

The returned status depends on what the adapter can safely observe:

| Condition | Status |
| --- | --- |
| The observation is fully represented, with no omissions. | `complete` |
| Useful observations are available, but omissions are reported. | `partial` |
| The source is recognized, but no safe useful result is available. | `unsupported` |
| I/O or resource processing fails. | `error` |

#### Omission reasons (`omissions`)

The following conditions prevent `complete`:

- Unknown envelopes or event variants.
- Known items or message content that are not supported.
- Malformed JSONL or oversized rows.
- Missing correlations or tool results, or duplicate call identifiers.
- Invalid relationship identifiers.
- Versions outside the compatibility boundary for the requested operation.

Known items that cannot be represented by the public event model still produce explicit omissions.

#### Failure cause (`error`)

Failures such as I/O and resource-processing failures are described by a structured `error`.\
See the common [error specification](cli-json-contract.md#errors) for fields and codes.

## Input limits

Codex adapter input has these limits:

| Input | Limit |
| --- | ---: |
| Logical history | 128 MiB |
| One history row | 4 MiB |
| Rollout segments | 32 |
| Header | 1 MiB |
| Discovered files | 100,000 |
| Normalized events | 100,000 |

These adapter input bounds apply in addition to the public [response and pagination bounds](cli-json-contract.md#resource-bounds).

## Output handling

Recorded content is returned without automatic redaction.\
It can contain secrets.\
Callers decide what to retain or share externally; see [recorded content handling](cli-json-contract.md#recorded-content-handling).

## Adapter specification for maintainers

Rollout JSONL is a version-sensitive adapter input, not a stable public API.\
Apply the common [adapter maintenance constraints](architecture.md#maintenance) when changing these rules.

### Record reading

#### Rollout discovery

Session transcripts are stored below `sessions`, and archived transcripts are stored below `archived_sessions`.

The adapter walks both transcript locations deterministically.\
It accepts only bounded regular files contained by the provider root, rejects symlinks and special files, and rechecks file and root identity when opening content.

#### Stored history profiles

**Accepted profiles**

The adapter distinguishes legacy, native paginated, and canonicalized legacy paginated history.\
The [adapter compatibility table](../internal/provider/codex/compatibility.go) defines the accepted version-label and history-mode combinations.

The native paginated profile covers the explicitly accepted labels from `0.144.5` onward.

The canonicalized legacy paginated profile covers the verified migrated labels `0.92.0`, `0.94.0`, `0.94.0-alpha.10`, `0.95.0-alpha.3`, `0.98.0`, `0.99.0-alpha.5`, `0.101.0`, `0.117.0`, `0.118.0`, `0.139.0`, `0.142.5`, and `0.144.2`.

**Structural validation**

The canonicalized legacy paginated profile requires contiguous ordinals and the verified migrated row vocabulary.

An unrecognized row, an unsupported history mode, or invalid ordinals leaves the affected observation unavailable; the header version alone does not establish compatibility.

Codex's [legacy-to-paginated migration](https://github.com/openai/codex/blob/94174e44cbc54cece45f6052328ca0c2cd7a8a2a/codex-rs/thread-store/src/local/rollout_migration/canonicalizer.rs) retains the header CLI version while rewriting the stored rows.

### Logical history construction

Safely resolved inherited and sliced histories are used consistently by `list`, `show`, `events`, and `verify`.

#### Current rollout selection

When more than one artifact represents the same thread, the adapter follows Codex's current rollout selection ordering by timestamp and rollout ID.\
If that ordering cannot distinguish the current artifact, content operations return `unsupported` instead of silently selecting one copy.

#### Inherited history resolution

For `history_base`, follow the referenced rollout ID and read only the inherited prefix ending at the recorded ordinal and byte offset.

#### Child-session history boundary

Use `subagent_history_start_ordinal` to exclude copied parent rows and retain only the selected child session's rows.

Rows before `subagent_history_start_ordinal` belong to the copied parent context and do not affect the child source's hint, events, verification, or interaction time.

### Source information generation

#### Identity

Identity discovery reads the first bounded JSONL record.

The `session_meta.payload.id` field is the provider-native thread identity, so moving an artifact between active and archived locations does not change its `source_ref`.

#### Version hint

The hint remains a change signal, not a verified source version.

**Hint calculation**

The selected history determines the hint and hashed data:

| History condition | Hint and hashed data |
| --- | --- |
| No inherited or sliced history | `stat_hash`: a domain-separated hash of the current file size and modification time. |
| Safely resolved inherited history or a subagent history boundary | `content_hash` in the `v3` Codex hint namespace: hash, in logical order, the selected header plus the header and included byte ranges of each inherited rollout that contributes rows. |

**Unresolved history**

An unresolved lineage has no hint.

**Algorithm namespaces**

The current-artifact statistics hint uses the Codex `v0` algorithm namespace.\
The `v0` and `v3` hint namespaces are independent of the public response schema.

#### Source relationships

Parent and fork relationships are emitted only from explicit `parent_thread_id` and `forked_from_id` metadata.

#### Latest interaction time

The latest timestamp of recognized message and tool rows becomes `last_interaction_at`.

`list` and `show` stream the bounded effective history to establish interaction time.

Only records in the [logical history](#logical-history-construction) contribute to this time.

##### Accepted versions and structural conditions

Interaction-time decoding additionally accepts syntactically valid versions at or after `0.155.0` when every included rollout matches the known `legacy` or native `paginated` structure.\
This forward-compatible boundary has no version ceiling: future paginated records require contiguous ordinals and every included row must match the known message, tool, response, lifecycle, or bookkeeping shape.\
Known event and response rows that can advance interaction time must retain the required top-level fields and JSON types of the known structure; additional fields remain allowed.

This structural compatibility applies only to `list` and `show`; `events` and `verify` retain the verified version list above.

##### Time classification by record type

The following records affect the latest source interaction time as shown.\
The accepted profile, version, and structural conditions above still apply.

| Record | Effect on source time |
| --- | --- |
| Completed `FileChange` and `CollabAgentToolCall` | Advance time as tool interactions. |
| `FunctionCallOutput` | Advance time as a tool result. |
| `SubAgentActivity` | Do not advance time; this is a child-agent status observation. |
| Completed `Extension` with verified `web.search` or `clock.sleep` kind | Advance time as a tool interaction. |
| Other `Extension` kinds | Leave source time unavailable because their interaction meaning is uncertain. |
| Completed `WebSearch`, verified raw `web_search_call` and `tool_search_call`, and `tool_search_output` results | Advance time. When raw and completed rows describe one operation, use only the maximum timestamp. |
| Completed `Sleep` in `0.144.2` | Advance time as a tool interaction. |
| Completed `Reasoning`, `Plan`, and `ContextCompaction`, and the `compacted` envelope in canonicalized legacy paginated history | Do not advance time. |
| Verified `world_state` snapshots in `0.139.0`, `0.142.5`, and `0.144.2` | Do not advance time; these are bookkeeping rows. |
| `token_usage_record` in `0.142.5` | Do not advance time; this is bookkeeping. |
| `inter_agent_communication_metadata` in `0.144.2` | Do not advance time; this is bookkeeping. |
| Later token accounting, lifecycle events, and provider errors | Do not advance time. |

For canonicalized legacy paginated rollouts, only the observed message, tool, search, and bookkeeping row types are classified.

##### Conditions that prevent a source time

Unknown rows or variants, malformed required bookkeeping fields, and future changes that do not preserve those structures leave the source time unavailable.

Missing or invalid timestamps, uncertain rows, unresolved history, and input limits prevent a source time rather than triggering a file-time fallback.

#### Recorded working directories

`list` and `show` read `session_meta.payload.cwd`, `turn_context.payload.cwd`, completed `CommandExecution.cwd`, and recorded legacy `local_shell_call.action.cwd`.\
These become common `session`, `turn`, and `tool` evidence, without parsing command strings.\
Directory extraction uses the existing interaction-time version/profile and row-structure boundary, including its structural forward compatibility; it does not expand `events` or `verify` support.

`CommandExecution.cwd` uses the `PathUri` format.\
The adapter decodes local `file:` URIs to absolute paths in the host's path syntax and also accepts recorded absolute paths.\
Remote authorities, opaque fallback URIs, and paths for another platform leave directory coverage partial.

The selected header and effective logical rows contribute locations.\
Copied parent context before a child boundary and excluded inherited suffixes do not contribute.\
Missing or invalid directories, unknown rows, unresolved history, and resource limits leave directory coverage partial.\
A failed history boundary discards observations from that invalid traversal.\
Codex Git URLs, branches, and commits do not identify a local clone.\
For deleted worktrees without a usable saved mapping, repository search can use remaining Git registrations; otherwise membership stays unknown.

The official [persisted protocol structures](https://github.com/openai/codex/blob/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/protocol.rs) define session and turn `cwd` and Git information.\
The official [item structures](https://github.com/openai/codex/blob/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/items.rs) define recorded command directories.\
The official [`PathUri` implementation](https://github.com/openai/codex/blob/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/utils/path-uri/src/lib.rs) defines their URI encoding.\
These adapter inputs are not stable public fields.

### Event generation

#### Event row selection

The adapter recognizes the `session_meta`, `event_msg`, and `response_item` rollout envelopes needed for the public observations.

The history profile determines which rows become public events:

| Profile | Rows used for public events |
| --- | --- |
| Legacy | `user_message` and `agent_message` events are canonical messages. Response tool calls and persisted output rows provide correlation evidence. |
| Native paginated | Completed `UserMessage`, `AgentMessage`, `CommandExecution`, `McpToolCall`, and `DynamicToolCall` items are canonical. Lower-level response rows are not emitted again. |
| Canonicalized legacy paginated | Completed message items are canonical. Legacy response rows provide tool call/result correlation; completed tool items are not emitted again. |

**Rows outside the public event model**

For `0.153.0`, `token_usage_record` is bookkeeping.\
An encountered `realtime_item` remains incomplete because its interaction meaning is not normalized.

#### Type-specific conversion

Convert each supported event type using the following rules.

##### Message events

Message events use the message rows selected for the history profile above.

##### Tool call events

**Category**

Public categories remain `shell`, `file_change`, `mcp`, and `tool`.

**Name**

Tool names use recorded `tool`; command items use their recorded variant name `CommandExecution`, and legacy local shell calls use `local_shell_call`.

**Action**

Commands emit `execute`, MCP and dynamic tools emit `invoke`, and known legacy patch calls emit `edit`.

**Input**

Legacy calls expose the recorded `name` and `arguments` or `input`; local shell calls expose their recorded `action` object, retaining command arrays and other input fields.\
Completed items expose `command` with recorded `cwd` and `interaction_input` when present, or MCP/dynamic `arguments`.

Older accepted rows missing an expected input field report `input_state: unavailable`, not inferred absence.

**Call identifier**

Tool call identifiers are deterministic adapter-owned hashes of the thread identity and explicit provider correlation identifier.

**Metadata**

Recorded MCP `server` and dynamic `namespace` values remain provider-specific metadata when present.

##### Tool result events

**Body**

Command results retain recorded `aggregated_output`, `stdout`, `stderr`, and `formatted_output`.\
One recorded field is returned as text; multiple fields are returned as distinct JSON fields.\
Recorded empty bodies remain distinct from absent bodies.

MCP text and `structuredContent` are both retained; dynamic `inputText` and legacy `input_text` results are also returned.

Multiple text blocks use an ordered JSON array; non-text and unsupported portions are reported as omissions.\
Known error fields are retained and, when output also exists, represented as JSON with `output` and `error`.

Verified native paginated `FunctionCallOutput` items also retain their bodies.

**Call relationship**

Correlation is resolved over the selected logical history; unmatched or duplicate-identifier results retain their bodies.

Item IDs of native paginated `FunctionCallOutput` records do not establish call correlation.\
These items remain standalone results.

**Outcome**

Outcomes use command `exit_code`, explicit dynamic `success`, MCP `isError`, and verified `status` semantics.\
If these do not establish an outcome, it is `unknown`; text such as `PASS` never establishes success.

Legacy output rows do not persist the internal success value, so the adapter retains their bodies with `outcome: unknown`.

Standalone native paginated `FunctionCallOutput` results also have unknown outcomes.

**Metadata**

The recorded name of a standalone native paginated `FunctionCallOutput` is retained in `tool_name` metadata.

##### Provider error events

Errors are normalized from explicit provider error events.

#### Event timestamp assignment

Events use their canonical rollout row's timestamp.\
A completed item gives its call and result the same time, not independently established start and end times.

### Verified-version calculation

**Content used for verification**

Logical-history verification streams bounded provider content through the common `VerifiedVersion` contract.

**Evidence encoding**

An unsliced rollout retains the path-free evidence name `rollout/primary.jsonl`; a logical history uses ordered, path-free header and history chunk names.

## Compatibility evidence

### Storage location evidence

OpenAI's [Codex troubleshooting documentation](https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting) documents the Codex home, active session transcript location, and archived session transcript location.\
JSONL row decoding remains an adapter-owned compatibility boundary rather than a claim that the persisted rows are a stable public API.

### Internal format evidence

**History format evidence**

The native paginated item and response persistence rules were checked against the corresponding official implementations, including [`0.147.0`](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/rollout/src/policy.rs) and [`0.148.0-alpha.9`](https://github.com/openai/codex/blob/rust-v0.148.0-alpha.9/codex-rs/rollout/src/policy.rs), and bounded structural inspection of saved rollouts.\
The original [`0.98.0`](https://github.com/openai/codex/blob/rust-v0.98.0/codex-rs/protocol/src/protocol.rs) and [`0.117.0`](https://github.com/openai/codex/blob/rust-v0.117.0/codex-rs/protocol/src/protocol.rs) formats did not include paginated ordinals.

#### Verified versions

**CLI builds**

The rollout compatibility evidence includes the official `openai/codex` tag [`rust-v0.149.1`](https://github.com/openai/codex/tree/rust-v0.149.1) at commit [`ff29a44391deccde0aba0f8390337d7f3c319ea4`](https://github.com/openai/codex/commit/ff29a44391deccde0aba0f8390337d7f3c319ea4).

The `0.153.0` boundary was checked against the official [`rust-v0.153.0` history implementation](https://github.com/openai/codex/blob/rust-v0.153.0/codex-rs/history/src/lib.rs).

The accepted versions were checked against their official [`0.152.0`](https://github.com/openai/codex/blob/rust-v0.152.0/codex-rs/protocol/src/items.rs), [`0.153.3`](https://github.com/openai/codex/blob/rust-v0.153.3/codex-rs/protocol/src/items.rs), [`0.153.4`](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/protocol/src/items.rs), and [`0.154.0`](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/protocol/src/items.rs) item definitions and against bounded, read-only structural inspection of saved rollouts; no private contents or identifiers were copied into this repository.

**App builds**

The App build versions were checked against their official [`0.154.0-alpha.6.2`](https://github.com/openai/codex/blob/rust-v0.154.0-alpha.6.2/codex-rs/protocol/src/items.rs) and [`0.155.0-alpha.9.2`](https://raw.githubusercontent.com/openai/codex/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/items.rs) item definitions and saved rollout structures, including both history modes observed for `0.155.0-alpha.9.2`.
