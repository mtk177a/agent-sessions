# ADR 0003: Separate provider discovery from version-sensitive decoding

- Status: Accepted
- Date: 2026-09-03

## Context

The initial provider-neutral CLI needs stable discovery boundaries without promoting provider-owned transcript formats into the public contract.

Provider documentation identifies storage roots, transcript locations, or export acquisition, but the JSONL rows and archive JSON graphs are version-sensitive implementation details.\
Starting a provider service or depending on a lifecycle hook would also add behavior that is unnecessary for read-only discovery.

## Decision

Provider adapters will separate documented location and lifecycle facts from version-sensitive artifact decoding.

For Codex:

- the documented provider home is the discovery root;
- `sessions` and `archived_sessions` under that home are the transcript discovery locations;
- the Codex App Server is not used as a read-only inspection path;
- transcript JSONL shapes remain an adapter-scoped compatibility boundary.

For Claude Code:

- the documented configuration root is the discovery root;
- the documented project transcript location under that root is the transcript discovery boundary;
- transcript JSONL shapes remain an adapter-scoped compatibility boundary.

For ChatGPT Data Export:

- the user-requested export ZIP is an explicit named source instance and is never auto-discovered or downloaded;
- the publicly documented export acquisition path establishes compatibility with the source type;
- conversation JSON graph shapes within the ZIP remain an adapter-scoped compatibility boundary rather than a stable OpenAI API;
- the archive snapshot identity remains distinct from each provider-native conversation identity.

For all providers:

- unknown artifact shapes fail closed as `partial`, `unsupported`, or `error` rather than being silently accepted as complete;
- provider-specific rows are normalized only inside the owning adapter;
- `SessionEnd` and other lifecycle hooks are not dependencies of initial discovery or correctness.

## Consequences

Documented root and transcript-location changes can be handled separately from row-decoding changes.\
The public schema does not inherit unstable provider JSONL fields, and read-only commands do not need to start provider services or install hooks.

Adapters must maintain explicit compatibility logic and synthetic tests for every provider shape they accept.\
An unknown shape may reduce completeness even when some safe information can still be returned.

## References

- Codex troubleshooting documentation: <https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting>
- Codex hooks documentation: <https://learn.chatgpt.com/ja-JP/docs/hooks>
- Claude Code session documentation: <https://code.claude.com/docs/en/sessions>
- Claude Code hooks documentation: <https://code.claude.com/docs/en/hooks>
- OpenAI Data Export documentation: <https://help.openai.com/en/articles/7260999-how-do-i-export-my-data>
