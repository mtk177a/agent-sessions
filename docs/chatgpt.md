# ChatGPT Data Export provider

The `chatgpt` provider adapter reads a user-requested ChatGPT Data Export ZIP and maps conversations into the provider-neutral stable `v1` CLI contract.

OpenAI documents how users request a Data Export and that the downloaded ZIP contains chat history.\
OpenAI does not document the archive's conversation JSON graph as a stable API, so every internal member name and field used for normalization remains an adapter-owned compatibility boundary.

## Acquisition and source selection

Request an export through ChatGPT **Settings → Data Controls → Export Data** or through OpenAI's Privacy Portal, then download it within the availability period described by OpenAI.

`agent-sessions` does not request, poll, discover, or download exports.\
Each archive must be selected as a named source instance by using `--provider chatgpt` with `--root`, or by adding an explicit configuration entry:

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

## Identity and verification

The archive and its conversations have different identities:

- the named source instance identifies the explicitly selected archive boundary;
- each provider-native conversation ID determines one logical source and `source_ref`;
- the SHA-256 of the whole ZIP is the `snapshot_hash` version hint shared by conversations in that snapshot;
- `verify` streams the whole ZIP through the common path-free `archive/export.zip` evidence name and the existing `provider-content-v0` algorithm.

Replacing the ZIP with a later export can preserve conversation identity while changing the snapshot and verified version.\
Paths and archive member names are locators, not conversation identities.

## Discovery and normalization

The compatibility decoder reads `conversations.json` and numbered `conversations-*.json` members in deterministic member-name order.\
It accepts a consistent `id` or `conversation_id` as provider-native conversation identity and rejects ambiguous duplicates.

For `events`, the adapter starts at the conversation's current node, follows parent links to the root, reverses that chain, and emits supported observations in chronological order.\
Text and multimodal-text string parts from `user` and `assistant` messages become normalized message events.

Nodes outside the active chain are omitted with `non_active_branch`.\
Tool-role results are omitted with `correlation_omitted` because the accepted export shape does not provide a stable provider-neutral call/result relationship at this boundary.\
Unknown active nodes or content are reported with `unknown_node`; useful remaining events return `partial`, while an observation with no safely usable event returns `unsupported`.\
A missing active node, a parent cycle, or an unusable graph returns `unsupported_format` rather than inventing an order or silently shortening the branch.

All normalized output passes through the common final redaction policy immediately before JSON encoding.\
Redaction retains `redaction_policy_version`, adds `output_redacted`, and prevents a result from remaining `complete`.

## Archive safety and bounds

The adapter opens the configured path as one regular file and does not extract any member.\
It rejects absolute, traversal, backslash, and drive-prefixed member paths, duplicate member names, encrypted members, symbolic links, and other special member types.

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

Malformed ZIP data, malformed JSON, duplicate members, unsafe members, source replacement, and resource-limit failures have distinct structured error codes.\
Recognized but incompatible JSON shapes return `unsupported` instead of being classified as malformed input.

Read-only commands create no extracted archive, cache, index, database, transcript mirror, consumer state, or background process.\
Synthetic tests generate ZIP and JSON inputs independently; no real Data Export content is committed.

## Public compatibility evidence

- [How do I export my ChatGPT history and data?](https://help.openai.com/en/articles/7260999-how-do-i-export-my-data) documents the export request and download process and states that chat history is included in the ZIP.
- [How can I view my conversation history in a ChatGPT EDU workspace?](https://help.openai.com/en/articles/20001279) identifies `conversations.json` or numbered conversation JSON files as export history files.

These references establish export acquisition and the presence of conversation history.\
They do not guarantee the internal JSON graph parsed by this adapter.
