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

Without inherited history, the list version hint remains a domain-separated hash of the current file size and modification time.\
With safely resolved inherited history, `content_hash` uses the `v1` Codex hint namespace and hashes the included byte ranges in rollout order with their rollout IDs and byte lengths; an unresolved lineage has no hint.\
The hint remains a change signal, not a verified source version.\
Verification hashes bounded provider content through the common `VerifiedVersion` contract, using the path-free evidence name `rollout/primary.jsonl`.

## Version-sensitive decoding

The decoder accepts verified rollout JSONL structures from Codex CLI `0.149.1`, `0.152.0`, `0.153.0`, `0.153.3`, `0.153.4`, and `0.154.0`, plus the observed Codex App builds `0.154.0-alpha.6.2` and `0.155.0-alpha.9.2`.\
The older boundary was checked against the official `openai/codex` tag [`rust-v0.149.1`](https://github.com/openai/codex/tree/rust-v0.149.1) at commit [`ff29a44391deccde0aba0f8390337d7f3c319ea4`](https://github.com/openai/codex/commit/ff29a44391deccde0aba0f8390337d7f3c319ea4).\
The `0.153.0` boundary was checked against the official [`rust-v0.153.0` history implementation](https://github.com/openai/codex/blob/rust-v0.153.0/codex-rs/history/src/lib.rs); its `token_usage_record` is bookkeeping, while an encountered `realtime_item` remains incomplete because its interaction meaning is not normalized.
The additional versions were checked against their official [`0.152.0`](https://github.com/openai/codex/blob/rust-v0.152.0/codex-rs/protocol/src/items.rs), [`0.153.3`](https://github.com/openai/codex/blob/rust-v0.153.3/codex-rs/protocol/src/items.rs), [`0.153.4`](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/protocol/src/items.rs), and [`0.154.0`](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/protocol/src/items.rs) item definitions and against bounded, read-only structural inspection of saved rollouts; no private contents or identifiers were copied into this repository.
The two App build versions were also checked against their official [`0.154.0-alpha.6.2`](https://github.com/openai/codex/blob/rust-v0.154.0-alpha.6.2/codex-rs/protocol/src/items.rs) and [`0.155.0-alpha.9.2`](https://raw.githubusercontent.com/openai/codex/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/items.rs) item definitions and saved rollout structures, including both history modes observed for `0.155.0-alpha.9.2`.

The adapter recognizes the `session_meta`, `event_msg`, and `response_item` rollout envelopes needed for the public observations.\
For legacy history, `user_message` and `agent_message` events are the canonical message rows, while response tool calls and their persisted output rows provide correlation evidence.\
Legacy output rows do not persist the internal success value, so the adapter omits the normalized result instead of guessing its outcome.\
For paginated history, completed `UserMessage`, `AgentMessage`, `CommandExecution`, `McpToolCall`, and `DynamicToolCall` items are canonical; lower-level response rows are not emitted again.

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

The latest timestamp of canonical message and tool rows becomes `last_interaction_at`.\
Completed `FileChange` and `CollabAgentToolCall` items are tool interactions, and `FunctionCallOutput` is a tool result; their timestamps advance it.\
`SubAgentActivity` is a child-agent status observation and does not advance it.\
Completed `Extension` items with verified `web.search` or `clock.sleep` kinds are tool interactions; other extension kinds remain uncertain.\
Later token accounting, lifecycle events, and provider errors do not advance it.\
For `history_base`, the adapter follows the referenced rollout ID and reads only the inherited prefix ending at the recorded ordinal and byte offset.\
Missing or invalid timestamps, uncertain rows, unresolved history, and input limits prevent a source time rather than triggering a file-time fallback.

## Completeness and limitations

Unknown envelopes, unknown event variants, unsupported known items or message content, malformed JSONL, oversized rows, omitted correlations or tool results, duplicate call identifiers, invalid relationship identifiers, and unverified CLI versions prevent `complete`.\
Useful observations return `partial`; a recognized source that cannot yield a safe useful result returns `unsupported`; I/O and resource failures return `error`.

Compressed `.jsonl.zst` rollouts are recognized but not decoded because the executable has no external compression dependency.\
For `events` and `verify`, rollouts with `history_base` still return only current-artifact observations and report the omitted inherited prefix.\
`verify` is partial; `events` is partial when useful current events exist and unsupported otherwise.

Interaction-time reading is limited to 128 MiB across the effective history, 4 MiB per row, and 32 rollout segments.\
For `events` and `verify`, artifacts remain limited to 64 MiB and rows to 1 MiB; headers remain limited to 1 MiB, discovered files to 100,000, and normalized events to 100,000.\
These are adapter input bounds in addition to the public response and pagination bounds.

Read-only commands create no index, cache, mirror, database, or provider application metadata update.\
The host file system may update access-time metadata when rollout files are read.\
Lifecycle hooks, preregistration, synchronization, and background ingestion are not discovery dependencies.

## Public location evidence

OpenAI's [Codex troubleshooting documentation](https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting) documents the Codex home, active session transcript location, and archived session transcript location.\
JSONL row decoding remains an adapter-owned compatibility boundary rather than a claim that the persisted rows are a stable public API.
