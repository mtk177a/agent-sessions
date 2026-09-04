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

Discovery reads the first bounded JSONL record only.\
The `session_meta.payload.id` field is the provider-native thread identity, so moving an artifact between active and archived locations does not change its `source_ref`.

When more than one artifact represents the same thread, the adapter follows Codex's current rollout selection ordering by timestamp and rollout ID.\
If that ordering cannot distinguish the current artifact, content operations return `unsupported` instead of silently selecting one copy.

The list version hint is a domain-separated hash of file size and modification time.\
Verification hashes bounded provider content through the common `VerifiedVersion` contract, using the path-free evidence name `rollout/primary.jsonl`.

## Version-sensitive decoding

The decoder is intentionally scoped to rollout JSONL written by Codex CLI `0.149.1`.\
Compatibility was checked against the official `openai/codex` tag [`rust-v0.149.1`](https://github.com/openai/codex/tree/rust-v0.149.1) at commit [`ff29a44391deccde0aba0f8390337d7f3c319ea4`](https://github.com/openai/codex/commit/ff29a44391deccde0aba0f8390337d7f3c319ea4), including the protocol, history, thread-store, and state extraction implementations.

The adapter recognizes the `session_meta`, `event_msg`, and `response_item` rollout envelopes needed for the public observations.\
For legacy history, `user_message` and `agent_message` events are the canonical message rows, while response tool calls and their persisted output rows provide correlation evidence.\
Legacy output rows do not persist the internal success value, so the adapter omits the normalized result instead of guessing its outcome.\
For paginated history, completed `UserMessage`, `AgentMessage`, `CommandExecution`, `McpToolCall`, and `DynamicToolCall` items are canonical; lower-level response rows are not emitted again.

Tool call identifiers are deterministic adapter-owned hashes of the thread identity and provider correlation identifier.\
Public categories are limited to `shell`, `file_change`, `mcp`, and `tool`; raw commands, arguments, tool payloads, and arbitrary provider tool names are not normalized into public structural fields.

Errors are normalized from explicit provider error events.\
Parent and fork relationships are emitted only from explicit `parent_thread_id` and `forked_from_id` metadata.

## Completeness and limitations

Unknown envelopes, unknown event variants, unsupported known items or message content, malformed JSONL, oversized rows, omitted correlations or tool results, duplicate call identifiers, invalid relationship identifiers, and unverified CLI versions prevent `complete`.\
Useful observations return `partial`; a recognized source that cannot yield a safe useful result returns `unsupported`; I/O and resource failures return `error`.

Compressed `.jsonl.zst` rollouts are recognized but not decoded because the executable has no external compression dependency.\
Rollouts with `history_base` return only current-artifact observations and explicitly report the omitted inherited prefix as partial.

Artifacts are limited to 64 MiB, individual rows and headers to 1 MiB, discovered files to 100,000, and normalized events to 100,000.\
These are adapter input bounds in addition to the public response and pagination bounds.

Read-only commands create no index, cache, mirror, database, or provider application metadata update.\
The host file system may update access-time metadata when rollout files are read.\
Lifecycle hooks, preregistration, synchronization, and background ingestion are not discovery dependencies.

## Public location evidence

OpenAI's [Codex troubleshooting documentation](https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting) documents the Codex home, active session transcript location, and archived session transcript location.\
JSONL row decoding remains an adapter-owned compatibility boundary rather than a claim that the persisted rows are a stable public API.
