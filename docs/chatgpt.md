# ChatGPT Data Export provider

Use this document for the `chatgpt` adapter to select an existing ChatGPT Data Export ZIP and understand which conversations and branches it exposes.\
The maintainer sections describe archive decoding and compatibility evidence; the [CLI reference](cli-json-contract.md) defines the common `v2` JSON fields.

## Select an export ZIP

Request an export through ChatGPT **Settings → Data Controls → Export Data** or through OpenAI's Privacy Portal, then download it within the availability period described by OpenAI.

`agent-sessions` does not request, poll, discover, or download exports.\
Each archive must be selected as a named source instance by using `--provider chatgpt` with `--root`, or by adding an explicit configuration entry:

```sh
agent-sessions list --provider chatgpt --source-instance chatgpt-export --root /fictional/chatgpt-export.zip
agent-sessions events --root /fictional/chatgpt-export.zip '<source_ref from list>'
```

```json
{
  "schema_version": "v1",
  "sources": [
    {
      "id": "chatgpt-personal",
      "provider": "chatgpt",
      "root": "/fictional/chatgpt-export.zip"
    }
  ]
}
```

The root is the exact ZIP file, not a directory.\
There is no environment or default root for this provider.

## Conversations, branches, and content

`events` returns text and multimodal-text string parts from user and assistant messages on the selected active conversation branch.\
Other branches are omitted with `non_active_branch`.

Readable tool-role results are also returned, but their call correlation and outcome cannot be established and are reported as omissions.\
Unknown content and unusable conversation branches also produce explicit incomplete observations.

Messages and tool results follow the common bounds without automatic content redaction.\
Readable string portions are retained, and excluded non-text or unsupported portions are reported.\
Content can contain secrets; consumers own retention and external sharing.

### Interaction timestamps

`list` and `show` expose `last_interaction_at` when the selected conversation branch establishes the latest interaction time.\
If they cannot establish it, they omit the field and return `partial` with `source_time_unavailable`.\
Emitted message events separately report their own `create_time` as `recorded_at`; missing or invalid values make `events` partial with `event_time_omitted`.

## Archive limits

The current input bounds are:

| Resource | Bound |
| --- | ---: |
| ZIP file | 4 GiB |
| Archive members | 50,000 |
| Declared uncompressed archive content | 8 GiB |
| One conversation JSON member | 64 MiB |
| All conversation JSON members | 2 GiB |
| Uncompressed-to-compressed member ratio | 50:1 |
| Conversations | 100,000 |
| Active-branch nodes | 100,000 |
| JSON nesting | 64 levels |

These are archive input limits; the common [response and pagination bounds](cli-json-contract.md#resource-bounds) still apply.\
The ZIP is read without extracting members to disk.

## Adapter behavior for maintainers

OpenAI documents how users request a Data Export and that the downloaded ZIP contains chat history.\
OpenAI does not document the archive's conversation JSON graph as a stable API, so every internal member name and field used for normalization remains an adapter-owned compatibility boundary.

Apply the common [adapter maintenance constraints](architecture.md#maintaining-provider-adapters) when changing these rules.

### Identity and verification

The archive and its conversations have different identities:

- the named source instance identifies the explicitly selected archive boundary;
- each provider-native conversation ID determines one logical source and `source_ref`;
- the SHA-256 of the whole ZIP is the `snapshot_hash` version hint shared by conversations in that snapshot;
- `verify` streams the whole ZIP through the common path-free `archive/export.zip` evidence name and the existing `provider-content-v0` algorithm.

Replacing the ZIP with a later export can preserve conversation identity while changing the snapshot and verified version.\
Paths and archive member names are locators, not conversation identities.

### Branch traversal and event normalization

For `events`, the adapter starts at the conversation's current node, follows parent links to the root, reverses that chain, and emits supported observations in chronological order.\
Text and multimodal-text string parts from `user` and `assistant` messages become normalized message events.

Nodes outside the active chain are omitted with `non_active_branch`.\
Tool-role results retain supported text content as standalone results with `correlation_state: unmatched`, `outcome: unknown`, and corresponding omissions.\
The accepted shape provides no stable call/result relationship; neither graph parents nor adjacent records imply correlation.\
Unknown active nodes or content are reported with `unknown_node`; useful remaining events return `partial`, while an observation with no safely usable event returns `unsupported`.\
A missing active node, a parent cycle, or an unusable graph returns `unsupported_format` rather than inventing an order or silently shortening the branch.

### Archive members and conversation identity

The compatibility decoder reads `conversations.json` and numbered `conversations-*.json` members in deterministic member-name order.\
It accepts a consistent `id` or `conversation_id` as provider-native conversation identity and rejects ambiguous duplicates.

### Interaction-time classification

For `last_interaction_at`, the adapter uses `create_time` on recognized `user` and `assistant` text or multimodal-text messages and recognized tool-role results on that same branch.\
It converts numeric Unix seconds exactly to a UTC timestamp and uses the latest value, including interactions after a resumed conversation.\
Assistant `thoughts` and `reasoning_recap`, other branches, conversation `update_time`, and ZIP file modification time do not advance the field.\
An invalid branch, an unknown potentially interactive record, a missing or invalid interaction time, or no interaction leaves the field absent; `list` aggregates the number of affected conversations in one omission.

### Archive validation

The adapter opens the configured path as one regular file and does not extract any member.\
It rejects absolute, traversal, backslash, and drive-prefixed member paths, duplicate member names, encrypted members, symbolic links, and other special member types.

Malformed ZIP data, malformed JSON, duplicate members, unsafe members, source replacement, and resource-limit failures have distinct structured error codes.\
Recognized but incompatible JSON shapes return `unsupported` instead of being classified as malformed input.

## Compatibility evidence

- [How do I export my ChatGPT history and data?](https://help.openai.com/en/articles/7260999-how-do-i-export-my-data) documents the export request and download process and states that chat history is included in the ZIP.
- [How can I view my conversation history in a ChatGPT EDU workspace?](https://help.openai.com/en/articles/20001279) identifies `conversations.json` or numbered conversation JSON files as export history files.

These references establish export acquisition and the presence of conversation history.\
They do not guarantee the internal JSON graph parsed by this adapter.
