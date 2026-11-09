# ChatGPT Data Export provider

Use this document to read a saved ChatGPT Data Export ZIP with the `chatgpt` adapter and understand its output.\
For common `v2` JSON fields, see the [CLI reference](cli-json-contract.md).

## Usage procedure

### 1. Prepare an export ZIP

Request and download an export before using the adapter.\
Follow OpenAI's [export instructions](https://help.openai.com/en/articles/7260999-how-do-i-export-my-data) for ChatGPT settings or the Privacy Portal, and download within the stated availability period.\
`agent-sessions` does not request, poll for, discover, or download exports.

### 2. Select the export ZIP

Select each archive as a named source instance.\
The root is the exact ZIP file, not its directory.\
This provider has no environment variable or default root.

Choose one of the following methods to select the ZIP and list its conversations.

#### Command options

Use `--provider chatgpt`, `--source-instance`, and `--root`:

```sh
agent-sessions list --provider chatgpt --source-instance chatgpt-export --root /fictional/chatgpt-export.zip
```

#### Configuration file

Add an explicit entry in the [configuration file](cli-json-contract.md#configuration).\
For example:

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

Then list conversations from the configured instance:

```sh
agent-sessions list --provider chatgpt --source-instance chatgpt-personal
```

### 3. Read a conversation

Copy `identity.source_ref` for the conversation from the `list` result into the placeholder below.\
If you selected the ZIP through command options, pass the same `--root` to this and later source operations:

```sh
agent-sessions events --root /fictional/chatgpt-export.zip '<source_ref from list>'
```

If you use the configured instance, omit `--root` from this command; the configuration supplies the location.

## Reading the output

### Returned data

The `data` field contains the information requested by the operation.\
The following sections describe each kind of returned data.

#### Source information

`list` and `show` return conversation information.\
They include `last_interaction_at` when the selected branch establishes the latest interaction time.\
Otherwise, they omit the field and return `partial` with `source_time_unavailable`.

#### Events

`events` returns supported observations from the selected active conversation branch.\
Other branches are omitted with `non_active_branch`.\
An unusable branch produces an explicit incomplete result; the adapter does not invent an order or silently shorten the branch.

Messages and tool results follow the common content bounds.\
Readable string portions are retained, and excluded non-text or unsupported portions are reported.

##### Messages

The adapter returns text and multimodal-text string parts from user and assistant messages.

##### Tool results

The adapter also returns readable tool-role results.\
It cannot establish their call correlation or outcome and reports these missing relationships and outcomes as omissions.

#### Verified version

`verify` returns a verified content version that covers the whole ZIP.

### Operation status

Use the status fields to identify complete observations, incomplete observations, unsupported input, and failures.

#### Status (`status`)

The returned status has these meanings:

| Status | Meaning |
| --- | --- |
| `complete` | The observation is fully represented. |
| `partial` | Useful information remains, but some information is omitted. |
| `unsupported` | The input is recognized, but no safely useful observation can be returned. |
| `error` | The operation failed; inspect `error` for the cause. |

Unknown content or nodes can leave useful events and return `partial`.\
If no safely usable event remains after omissions, the observation returns `unsupported`.\
Recognized but incompatible JSON structures also return `unsupported`, rather than a malformed-input error.

#### Omission reasons (`omissions`)

These codes explain provider-specific omissions:

| Code | Condition |
| --- | --- |
| `non_active_branch` | Nodes outside the selected branch are omitted. |
| `unknown_node` | An active node or its content is unknown. |
| `unsupported_format` | The archive JSON structure or conversation graph is unsupported, including a missing active node or a parent cycle. The affected operation returns `unsupported`. |
| `source_time_unavailable` | `list` or `show` cannot establish the latest interaction time. They omit `last_interaction_at` and return `partial`. |
| `event_time_omitted` | The original message's `create_time` is missing or invalid; `events` returns `partial`. |

Tool results also report omissions for their unknown call relationships and outcomes.

#### Failure cause (`error`)

Malformed ZIP data, malformed JSON, duplicate members, unsafe members, source replacement, and resource-limit failures have distinct structured error codes.\
See the common [error specification](cli-json-contract.md#structured-errors-and-exit-codes) for the error fields and codes.

## Input limits

The adapter applies these archive input limits:

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

The common [response and pagination bounds](cli-json-contract.md#resource-bounds) also apply.

## Output handling

The adapter returns recorded content without automatic redaction.\
Content can contain secrets.\
Callers decide what to retain or share externally; see [content safety](cli-json-contract.md#content-safety).

## Adapter specification for maintainers

OpenAI documents export acquisition and the presence of chat history in the ZIP.\
It does not document the internal conversation JSON graph as a stable API.\
The adapter therefore owns compatibility checks for the internal member names and fields that it uses for normalization.

Apply the common [adapter maintenance constraints](architecture.md#maintaining-provider-adapters) when changing these rules.

### Archive reading

#### ZIP file checks

The adapter opens the configured path as one regular file.\
It reads the ZIP without extracting members to disk.

#### Archive member reading

**Member checks**

The adapter rejects:

- Absolute, traversal, backslash, and drive-prefixed member paths.
- Duplicate member names.
- Encrypted members.
- Symbolic links and other special member types.

**Conversation JSON selection**

The decoder reads `conversations.json` and numbered `conversations-*.json` members in deterministic member-name order.

### Source information generation

#### Identity

The named source instance identifies the explicitly selected archive boundary.\
Within that instance, each provider-native conversation ID determines one logical source and `source_ref`.

The adapter accepts a consistent `id` or `conversation_id` as the provider-native conversation identity and rejects ambiguous duplicates.\
Paths and archive member names are locators, not conversation identities.

Replacing the ZIP with a later export can preserve conversation identity while changing the snapshot and verified version.

#### Version hint

The SHA-256 of the whole ZIP is the `snapshot_hash` version hint.\
All conversations in that snapshot share the hint.

#### Latest interaction time

**Time calculation**

For `last_interaction_at`, the adapter uses `create_time` on recognized `user` and `assistant` text or multimodal-text messages and recognized tool-role results on the same branch.\
It converts numeric Unix seconds exactly to a UTC timestamp.\
It uses the latest value, including interactions after a resumed conversation.

**Excluded observations**

The following do not advance `last_interaction_at`:

- Assistant `thoughts` and `reasoning_recap`.
- Other branches.
- Conversation `update_time`.
- ZIP file modification time.

**Unavailable time**

The field remains absent in these cases:

- The branch is invalid.
- A potentially interactive record is unknown.
- An interaction time is missing or invalid.
- There is no interaction.

`list` aggregates the number of affected conversations in one omission.

### Event generation

#### Conversation branch selection

For `events`, the adapter selects the active branch in this order:

1. Start at the conversation's current node.
2. Follow parent links to the root.
3. Reverse that chain and emit supported observations in chronological order.

Nodes outside the active chain are omitted with `non_active_branch`.

A missing active node, a parent cycle, or an unusable graph returns `unsupported_format`.\
The adapter does not invent an order or silently shorten the branch.

#### Type-specific conversion

The following sections define the conversion for each supported event type.

Unknown active nodes or content are reported with `unknown_node`.\
Useful remaining events return `partial`; an observation with no safely usable event returns `unsupported`.

##### Message events

Text and multimodal-text string parts from `user` and `assistant` messages become normalized message events.

##### Tool result events

**Body**

Tool-role results retain supported text content as standalone results.

**Call relationship**

Results use `correlation_state: unmatched`, with a corresponding omission.\
The accepted structure provides no stable call/result relationship.\
Neither graph parents nor adjacent records imply correlation.

**Outcome**

Results use `outcome: unknown`, with a corresponding omission.

#### Event timestamp assignment

The adapter sets `recorded_at` from the original message's `create_time`.\
Missing or invalid values make `events` partial with `event_time_omitted`.

### Verified-version calculation

`verify` streams the whole ZIP for content verification.\
Verification uses the path-free evidence name `archive/export.zip` and the common `provider-content-v0` algorithm.

## Compatibility evidence

The following official references describe exports and their conversation files:

- [Exporting your ChatGPT history and data](https://help.openai.com/en/articles/7260999-how-do-i-export-my-data) documents the export request and download process and states that chat history is included in the ZIP.
- [Exporting data from a ChatGPT Edu workspace](https://help.openai.com/en/articles/20001279) identifies `conversations.json` or numbered conversation JSON files as export history files.

These references establish export acquisition and the presence of conversation history.\
They do not guarantee the internal JSON graph parsed by this adapter.
