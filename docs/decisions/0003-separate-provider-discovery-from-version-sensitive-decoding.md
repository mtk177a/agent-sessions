# ADR 0003: Separate provider discovery from version-sensitive decoding

- Status: Accepted
- Date: 2026-09-03

## Context

### Limits of discovery

The initial provider-neutral CLI needs stable discovery boundaries without making provider-owned transcript formats part of the public contract.

Provider documentation identifies storage roots, transcript locations, or how to obtain an export.\
The JSONL rows and conversation JSON graphs in archives are implementation details that can change between versions.\
Finding a record does not establish that the adapter can decode its format.

### Unnecessary dependencies

Starting a provider service or depending on a lifecycle hook would add behavior that read-only discovery does not need.

## Decision

Provider adapters will separate documented location and lifecycle facts from version-sensitive artifact decoding.

### Common principles

#### Record format support

- Provider-specific rows are normalized only inside the owning adapter.
- Adapters must maintain explicit compatibility logic and synthetic tests for every provider shape they accept.
- Unknown artifact shapes fail closed as `partial`, `unsupported`, or `error` rather than being silently accepted as complete.

#### Independence from lifecycle hooks

`SessionEnd` and other lifecycle hooks are not dependencies of initial discovery or correctness.

### Provider-specific rules

#### Codex

- **Discovery locations**

  The documented provider home is the discovery root.\
  `sessions` and `archived_sessions` under that home are the transcript discovery locations.

- **Read interface**: The Codex App Server is not used as a read-only inspection path.

- **Record format**: Transcript JSONL shapes remain an adapter-scoped compatibility boundary.

#### Claude Code

- **Discovery locations**

  The documented configuration root is the discovery root.\
  The documented project transcript location under that root is the transcript discovery boundary.

- **Record format**: Transcript JSONL shapes remain an adapter-scoped compatibility boundary.

#### ChatGPT Data Export

- **Input selection**: The user-requested export ZIP is an explicit named source instance and is never auto-discovered or downloaded.
- **Export acquisition**: The publicly documented export acquisition path establishes compatibility with the source type.
- **Record format**: Conversation JSON graph shapes within the ZIP remain an adapter-scoped compatibility boundary rather than a stable OpenAI API.
- **Identity**: The archive snapshot identity remains distinct from each provider-native conversation identity.

## Consequences

### Benefits

- Documented root and transcript-location changes can be handled separately from row-decoding changes.
- The public schema does not inherit unstable provider JSONL fields.
- Read-only commands do not need to start provider services or install hooks.

### Accepted costs

- Each supported provider format requires maintenance of compatibility logic and synthetic tests.
- An unknown shape may reduce completeness even when some safe information can still be returned.

## References

- Codex troubleshooting documentation: <https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting>
- Codex hooks documentation: <https://learn.chatgpt.com/ja-JP/docs/hooks>
- Claude Code session documentation: <https://code.claude.com/docs/en/sessions>
- Claude Code hooks documentation: <https://code.claude.com/docs/en/hooks>
- OpenAI Data Export documentation: <https://help.openai.com/en/articles/7260999-how-do-i-export-my-data>
