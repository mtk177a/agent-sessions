# Codex provider

The `codex` provider adapter reads Codex rollout artifacts directly from a Codex home without starting the Codex App Server.

It maps the compatibility boundary below into the provider-neutral stable `v1` CLI contract.\
The rollout JSONL format remains adapter-internal and is not part of that public schema.

## Discovery boundary

Codex documents `$CODEX_HOME` as its data directory and uses `~/.codex` when that variable is unset.\
Session transcripts are stored below `sessions`, and archived transcripts are stored below `archived_sessions`.

An explicit `--root` or configured source root identifies the Codex home, not either transcript subdirectory.\
Source resolution follows the common precedence of explicit root, configured source instance, `$CODEX_HOME`, and documented default home.

The adapter walks both transcript locations deterministically.\
It accepts only bounded regular files contained by the provider root, rejects symlinks and special files, and rechecks file and root identity when opening content.

Identity discovery reads the first bounded JSONL record; `list` and `show` stream the bounded effective history to establish interaction time.\
The `session_meta.payload.id` field is the provider-native thread identity, so moving an artifact between active and archived locations does not change its `source_ref`.

When more than one artifact represents the same thread, the adapter follows Codex's current rollout selection ordering by timestamp and rollout ID.\
If that ordering cannot distinguish the current artifact, content operations return `unsupported` instead of silently selecting one copy.

Without inherited or sliced history, the list version hint remains a domain-separated hash of the current file size and modification time.\
With safely resolved inherited history or a subagent history boundary, `content_hash` uses the `v2` Codex hint namespace and hashes the selected header and included byte ranges in logical order; an unresolved lineage has no hint.\
Rows before `subagent_history_start_ordinal` belong to the copied parent context and do not affect the child source's hint, events, verification, or interaction time.\
The hint remains a change signal, not a verified source version.\
Logical-history verification streams bounded provider content through the common `VerifiedVersion` contract.\
An unsliced rollout retains the path-free evidence name `rollout/primary.jsonl`; a logical history uses ordered, path-free header and history chunk names.

## Version-sensitive decoding

The decoder accepts the verified mode and structure combinations for the 32 observed version labels from `0.92.0` through `0.155.0-alpha.9.2`.\
The older boundary was checked against the official `openai/codex` tag [`rust-v0.149.1`](https://github.com/openai/codex/tree/rust-v0.149.1) at commit [`ff29a44391deccde0aba0f8390337d7f3c319ea4`](https://github.com/openai/codex/commit/ff29a44391deccde0aba0f8390337d7f3c319ea4).\
The `0.153.0` boundary was checked against the official [`rust-v0.153.0` history implementation](https://github.com/openai/codex/blob/rust-v0.153.0/codex-rs/history/src/lib.rs); its `token_usage_record` is bookkeeping, while an encountered `realtime_item` remains incomplete because its interaction meaning is not normalized.
The additional versions were checked against their official [`0.152.0`](https://github.com/openai/codex/blob/rust-v0.152.0/codex-rs/protocol/src/items.rs), [`0.153.3`](https://github.com/openai/codex/blob/rust-v0.153.3/codex-rs/protocol/src/items.rs), [`0.153.4`](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/protocol/src/items.rs), and [`0.154.0`](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/protocol/src/items.rs) item definitions and against bounded, read-only structural inspection of saved rollouts; no private contents or identifiers were copied into this repository.
The two App build versions were also checked against their official [`0.154.0-alpha.6.2`](https://github.com/openai/codex/blob/rust-v0.154.0-alpha.6.2/codex-rs/protocol/src/items.rs) and [`0.155.0-alpha.9.2`](https://raw.githubusercontent.com/openai/codex/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/items.rs) item definitions and saved rollout structures, including both history modes observed for `0.155.0-alpha.9.2`.

The native paginated profile covers the verified labels from `0.144.5` onward, including the previously supported versions and observed alpha builds.\
Its item and response persistence rules were checked against the corresponding official implementations, including [`0.147.0`](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/rollout/src/policy.rs) and [`0.148.0-alpha.9`](https://github.com/openai/codex/blob/rust-v0.148.0-alpha.9/codex-rs/rollout/src/policy.rs), and bounded structural inspection of saved rollouts.\
The original [`0.98.0`](https://github.com/openai/codex/blob/rust-v0.98.0/codex-rs/protocol/src/protocol.rs) and [`0.117.0`](https://github.com/openai/codex/blob/rust-v0.117.0/codex-rs/protocol/src/protocol.rs) formats did not include paginated ordinals.\
Codex's [legacy-to-paginated migration](https://github.com/openai/codex/blob/94174e44cbc54cece45f6052328ca0c2cd7a8a2a/codex-rs/thread-store/src/local/rollout_migration/canonicalizer.rs) retains the header CLI version while rewriting the stored rows.\
The canonicalized legacy paginated profile covers the verified migrated labels `0.92.0`, `0.94.0`, `0.94.0-alpha.10`, `0.95.0-alpha.3`, `0.98.0`, `0.99.0-alpha.5`, `0.101.0`, `0.117.0`, `0.118.0`, `0.139.0`, `0.142.5`, and `0.144.2`.\
It requires contiguous ordinals and the verified migrated row vocabulary.\
An unrecognized row, an unsupported history mode, or invalid ordinals leaves the affected observation unavailable; the header version alone does not establish compatibility.

The adapter recognizes the `session_meta`, `event_msg`, and `response_item` rollout envelopes needed for the public observations.\
For legacy history, `user_message` and `agent_message` events are the canonical message rows, while response tool calls and their persisted output rows provide correlation evidence.\
Legacy output rows do not persist the internal success value, so the adapter omits the normalized result instead of guessing its outcome.\
For native paginated history, completed `UserMessage`, `AgentMessage`, `CommandExecution`, `McpToolCall`, and `DynamicToolCall` items are canonical; lower-level response rows are not emitted again.\
For canonicalized legacy paginated history, completed message items are canonical while legacy response rows provide tool call and result correlation; completed tool items are not emitted again.

Tool call identifiers are deterministic adapter-owned hashes of the thread identity and provider correlation identifier.\
Public categories are limited to `shell`, `file_change`, `mcp`, and `tool`; raw commands, arguments, tool payloads, and arbitrary provider tool names are not normalized into public structural fields.

Completed command items emit `execute`; completed MCP and dynamic tool items emit `invoke`.\
Known legacy shell and patch calls emit `execute` and `edit`, while other correlated calls emit `invoke`.\
For completed command items, the excerpt reader prefers non-empty `aggregated_output`, then `stdout` and `stderr`, then `formatted_output`.\
For completed MCP and dynamic tool items, it reads text content blocks and excludes structured or non-text content.\
Only lines permitted by the common safe excerpt policy appear in the public result; omitted content and unsupported body shapes reduce completeness.\
Legacy persisted output still cannot produce a normalized result because it lacks a safe success value.

Errors are normalized from explicit provider error events.\
Parent and fork relationships are emitted only from explicit `parent_thread_id` and `forked_from_id` metadata.

The latest timestamp of recognized message and tool rows becomes `last_interaction_at`.\
Completed `FileChange` and `CollabAgentToolCall` items are tool interactions, and `FunctionCallOutput` is a tool result; their timestamps advance it.\
`SubAgentActivity` is a child-agent status observation and does not advance it.\
Completed `Extension` items with verified `web.search` or `clock.sleep` kinds are tool interactions; other extension kinds remain uncertain.\
Completed `WebSearch` items, verified raw `web_search_call` and `tool_search_call` rows, and `tool_search_output` results also advance time; when raw and completed rows describe one operation, the time calculation still takes only the maximum timestamp.\
The `0.144.2` completed `Sleep` item is a tool interaction.\
For canonicalized legacy paginated rollouts, only the observed message, tool, search, and bookkeeping row types are classified; completed `Reasoning`, `Plan`, and `ContextCompaction` and the `compacted` envelope do not advance time.\
Verified `world_state` snapshots in `0.139.0`, `0.142.5`, and `0.144.2`, `token_usage_record` accounting in `0.142.5`, and `inter_agent_communication_metadata` in `0.144.2` are bookkeeping rows and do not advance time.\
Later token accounting, lifecycle events, and provider errors do not advance it.\
For `history_base`, the adapter follows the referenced rollout ID and reads only the inherited prefix ending at the recorded ordinal and byte offset.\
For `subagent_history_start_ordinal`, it excludes copied parent rows and retains only the selected child session's rows.\
Missing or invalid timestamps, uncertain rows, unresolved history, and input limits prevent a source time rather than triggering a file-time fallback.

## Completeness and limitations

Unknown envelopes, unknown event variants, unsupported known items or message content, malformed JSONL, oversized rows, omitted correlations or tool results, duplicate call identifiers, invalid relationship identifiers, and unverified CLI versions prevent `complete`.\
Useful observations return `partial`; a recognized source that cannot yield a safe useful result returns `unsupported`; I/O and resource failures return `error`.

Compressed `.jsonl.zst` rollouts are recognized but not decoded because the executable has no external compression dependency.\
Safely resolved inherited and sliced histories are used consistently by `list`, `show`, `events`, and `verify`.\
Known items that cannot be represented by the public event model still produce explicit omissions.

Codex logical-history reading is limited to 128 MiB, 4 MiB per row, and 32 rollout segments.\
Headers remain limited to 1 MiB, discovered files to 100,000, and normalized events to 100,000.\
These are adapter input bounds in addition to the public response and pagination bounds.

Read-only commands create no index, cache, mirror, database, or provider application metadata update.\
The host file system may update access-time metadata when rollout files are read.\
Lifecycle hooks, preregistration, synchronization, and background ingestion are not discovery dependencies.

## Public location evidence

OpenAI's [Codex troubleshooting documentation](https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting) documents the Codex home, active session transcript location, and archived session transcript location.\
JSONL row decoding remains an adapter-owned compatibility boundary rather than a claim that the persisted rows are a stable public API.
