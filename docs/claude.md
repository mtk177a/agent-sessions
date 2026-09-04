# Claude Code provider

The `claude` provider adapter reads Claude Code session records directly from a Claude Code configuration root without starting Claude Code.

It maps the compatibility boundary below into the provider-neutral stable `v1` CLI contract.\
The transcript JSONL format remains adapter-internal and is not part of that public schema.

## Discovery boundary

Claude Code uses `~/.claude` as its default configuration directory and supports overriding that directory with `CLAUDE_CONFIG_DIR`.\
Main session transcripts are stored at `projects/<project>/<session-id>.jsonl` below that root.

An explicit `--root` or configured source root identifies the Claude Code configuration root, not a project directory or transcript.\
Source resolution follows the common precedence of explicit root, configured source instance, `CLAUDE_CONFIG_DIR`, and the documented default root.

The adapter walks `projects` deterministically.\
It accepts only bounded regular files contained by the provider root, rejects symlinks and special files, and rechecks file and root identity when opening content.

The provider-native session identity comes from a validated `sessionId` field in the transcript.\
The filename is used only as a consistency check, so moving a transcript between project locators does not change its `source_ref`.

If more than one artifact contains the same provider-native session identity, the adapter does not guess which copy is current.\
`list` and `show` report the ambiguity, while content operations return `unsupported`.

The list version hint is a domain-separated hash of the artifact count, sizes, and modification times.\
Verification hashes the bounded main transcript through the common `VerifiedVersion` contract using the path-free evidence name `transcript/primary.jsonl`.

## Version-sensitive decoding

The decoder is intentionally scoped to internal transcript JSONL verified for Claude Code `2.1.177` and `2.1.228`.\
Claude Code documents the transcript location but states that each line's internal shape can change between releases, so a different row version prevents `complete` rather than being accepted speculatively.

Compatibility for `2.1.228` was checked against its official Claude Code distribution and the documented transcript, hook, status-line, and subagent location contracts.\
Compatibility for `2.1.177` was checked read-only against private provider-owned records and is covered by independently constructed synthetic fixtures.\
Only structural properties were inspected, and no private contents or identifiers were copied into this repository.

The adapter accepts `user`, `assistant`, `system`, and `attachment` rows at the verified version.\
It normalizes public user and assistant text, `tool_use` calls, explicitly correlated `tool_result` blocks, and explicit provider API errors.

Known bookkeeping rows are not public observations and are ignored.\
Unknown rows, unknown content blocks, attachments, reasoning blocks, internal user metadata, and unsupported system observations are reported as omissions.

Tool call identifiers are deterministic adapter-owned hashes of the session identity and provider correlation identifier.\
Results are related to calls only through the explicit `tool_use_id` value.

Duplicate calls, duplicate results, unmatched results, and calls without persisted results prevent `complete`.\
The result `success` value is derived only from the verified `tool_result.is_error` semantics: absent or `false` means success, and `true` means failure.

Public tool categories are limited to `shell`, `filesystem`, `file_change`, `mcp`, and `tool`.\
Raw commands, tool input, tool output, arbitrary provider tool names, provider correlation identifiers, and absolute paths are not exposed as public structural values.

## Subagents and sidecars

Claude Code documents subagent transcripts at `projects/<project>/<session-id>/subagents/agent-<agent-id>.jsonl`.\
The current distribution also writes adjacent metadata sidecars for these artifacts.

The adapter recognizes only this contained path, applies the same regular-file and root-containment checks, and bounds sidecar decoding.\
Missing, orphaned, malformed, or oversized sidecars are reported explicitly.

Subagent artifacts are not exposed as separate logical sources in this compatibility boundary.\
The verified transcript and sidecar records do not provide an independent provider-owned subagent identity field, and filename-derived identity or path-derived relationships would violate the common identity contract.

The main transcript is the only verification evidence for a main source.\
Subagent transcripts and sidecars therefore do not silently change a main source's verified content version.

## Completeness and bounds

Safe and fully represented observations return `complete`.\
Useful observations with omissions return `partial`; a recognized source with no safely useful observation returns `unsupported`; I/O, containment, replacement-race, or resource failures return `error`.

Artifacts are limited to 64 MiB, identity prefixes, JSONL rows, and sidecars to 1 MiB, discovered files to 100,000, and normalized events to 100,000.\
These provider-input bounds are in addition to the public response and pagination bounds.

Read-only commands create no index, cache, mirror, database, or provider application metadata update.\
Synthetic integration tests compare the provider tree's file set, type and mode, size, modification time, and content hash before and after representative `list`, `show`, `events`, and `verify` operations.

The host file system may update access-time metadata when transcript files are read.\
Lifecycle hooks, status-line integration, session resume, Agent SDK access, `/export`, `/insights`, synchronization, and background ingestion are not discovery or correctness dependencies.

## Public location evidence

- [Sessions](https://code.claude.com/docs/en/sessions) documents transcript persistence and the `projects/<project>/<session-id>.jsonl` location.
- [Environment variables](https://code.claude.com/docs/en/env-vars) documents `CLAUDE_CONFIG_DIR` and the default configuration directory behavior.
- [Hooks](https://code.claude.com/docs/en/hooks) documents the `session_id` and `transcript_path` lifecycle inputs.
- [Subagents](https://code.claude.com/docs/en/sub-agents) documents the subagent transcript location.

These references establish the location and lifecycle boundary.\
They do not make the internal JSONL entry shape a stable public API.
