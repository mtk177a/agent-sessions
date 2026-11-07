# Claude Code provider

Use this document to select a Claude Code configuration root and check which saved records the tool can read.\
The `claude` adapter reads these records directly without starting Claude Code.

For common `v2` JSON fields, see the [CLI reference](cli-json-contract.md).

<a id="select-a-claude-code-root"></a>

## Claude Code root selection

**Directory to select**

Use `--root` or a configured source root to select the Claude Code configuration directory.\
Do not select a project directory or an individual transcript.\
Claude Code stores main session transcripts at `projects/<project>/<session-id>.jsonl` below this root.

For example, list sessions from a specified root:

```sh
agent-sessions list --provider claude --root /fictional/claude-root
```

**Default and selection precedence**

The tool selects the root in this order:

1. Explicit `--root`.
2. Configured source instance.
3. `CLAUDE_CONFIG_DIR`.
4. Claude Code's documented default, `~/.claude`.

Claude Code also supports `CLAUDE_CONFIG_DIR` to override its default configuration directory.

**Using the same root in later commands**

For a nonstandard root, pass it to later source operations or configure a named instance.\
See [configuration](cli-json-contract.md#configuration).

## Supported records and limitations

### Support by operation

Support for interaction time differs from support for event decoding and format verification:

| Operations | Supported versions and conditions |
| --- | --- |
| `list`, `show`: interaction time | Verified versions `2.1.177`, `2.1.228`, and `2.1.260`. Also syntactically valid versions at or after `2.1.260` when every row retains a known structure, with no version ceiling. |
| `events`, `verify`: event decoding and format verification | Verified internal transcript JSONL structures for `2.1.177`, `2.1.228`, and `2.1.260`. Other versions remain incomplete. |

Claude Code documents the transcript location, but each line's internal structure can change between releases.\
The adapter does not accept unverified structures as complete.\
Forward-compatible interaction-time decoding does not extend verified `events` or `verify` support.

Unknown potentially interactive rows or unusable timestamps prevent a source time.\
See [source interaction time](#source-interaction-time) for the detailed rules.

<a id="subagent-records"></a>

### Subagent record limitations

The adapter recognizes subagent artifacts but does not expose them as independent sources.\
Only the main transcript contributes verification evidence for a main source.\
See [subagent record handling](#subagent-record-handling) for the locations and the reason for this limit.

<a id="incomplete-observations-and-input-limits"></a>

### Incomplete results and failures

The returned status depends on what the adapter can safely observe:

| Condition | Status |
| --- | --- |
| Safe observation that is fully represented | `complete` |
| Useful observation with omissions | `partial` |
| Recognized source with no safely useful observation | `unsupported` |
| I/O, root-containment, replacement-race, or resource failure | `error` |

### Input limits

The adapter applies these limits to provider input:

| Input | Limit |
| --- | --- |
| Artifact | 64 MiB |
| Prefix read to establish identity | 1 MiB |
| JSONL row | 1 MiB |
| Sidecar | 1 MiB |
| Discovered files | 100,000 |
| Normalized events | 100,000 |

These limits apply in addition to the public [response and pagination limits](cli-json-contract.md#resource-bounds).

## Adapter behavior for maintainers

The following sections define how the adapter identifies records and converts their contents.\
Internal JSONL structures depend on the provider version; they are not stable public fields.\
Apply the common [adapter maintenance constraints](architecture.md#maintaining-provider-adapters) when changing these rules.

<a id="discovery-identity-and-content-versions"></a>

### Record discovery and source identity

**Locations and file safety**

The adapter walks `projects` in a deterministic order.\
It accepts only regular files within the size limits and the provider root.\
It rejects symlinks and special files, and rechecks file and root identity when opening content.

**Session IDs and source references**

The provider-native session identity comes from a validated `sessionId` field in the transcript.\
The filename is used only as a consistency check.\
Moving a transcript between project locations does not change its `source_ref`.

**Multiple records for the same session**

If more than one artifact contains the same provider-native session identity, the adapter does not guess which copy is current.\
`list` and `show` report the ambiguity, while content operations return `unsupported`.

### Version hints for change detection

The list version hint is a domain-separated hash of the artifact count, sizes, and modification times.\
For the distinction between a hint and a verified version, see [change detection and content verification](architecture.md#change-detection-and-content-verification).

### Content verification

**Records to verify**

Verification hashes the main transcript within the input limits through the common `VerifiedVersion` contract.\
The main transcript is the only verification evidence for a main source.\
Changes to subagent transcripts and sidecars therefore do not change a main source's verified content version.

**Evidence name**

The main transcript uses the path-free evidence name `transcript/primary.jsonl`.

<a id="tool-calls-results-and-event-timestamps"></a>

### Event normalization

Each entire tool input or result is bounded to 64 KiB.\
The adapter returns recorded content without automatic redaction.

<a id="messages-and-omitted-content"></a>

#### Accepted rows and omitted content

At verified versions, the adapter accepts `user`, `assistant`, `system`, and `attachment` rows.\
It normalizes public user and assistant text and the tool and error records described below.

Known bookkeeping rows are not public observations and are ignored.\
The adapter reports omissions for:

- Unknown rows and unknown content blocks.
- Attachments.
- Reasoning blocks.
- Internal user metadata.
- Unsupported system observations.

#### Tool call names, categories, and inputs

**Names and classification**

The adapter returns the recorded `name` for each `tool_use` call.\
Calls use these public categories: `shell`, `filesystem`, `file_change`, `mcp`, and `tool`.\
Known tool names map to `execute`, `read`, `search`, `write`, or `edit`; other calls map to `invoke`.

**Recorded input**

The adapter returns object-valued `input`.\
It reports input availability as follows:

| Recorded input | State |
| --- | --- |
| Object, including an empty object | `available` |
| Missing field | `absent` |
| Unsupported value | `unsupported` |

#### Tool result bodies

The adapter returns string and text-block `tool_result` bodies without line filtering.\
Multiple text blocks become an ordered JSON array.\
If non-text content is excluded, the adapter reports the omission and retains readable text.

#### Call and result relationships

Tool call identifiers are deterministic, adapter-owned hashes of the session identity and provider correlation identifier.\
Results are related to calls only through the explicit `tool_use_id` value.\
An explicit, unique relationship can connect records in either order.

Duplicate calls and results, unmatched results, and calls without persisted results prevent `complete`.\
Readable bodies are retained.

#### Tool outcomes

The adapter determines result `outcome` from the verified meaning of `tool_result.is_error`:

| `tool_result.is_error` | Outcome |
| --- | --- |
| Absent or `false` | Success |
| `true` | Failure |
| Invalid value | Unknown; retain the body |

#### Provider errors

The adapter normalizes explicit provider API errors.

#### Individual event timestamps

Events use their transcript row's timestamp.\
Events from one row therefore share `recorded_at`.

<a id="interaction-time-classification"></a>

### Source interaction time

The latest timestamp of supported user and assistant message or tool rows becomes `last_interaction_at`.

**Supported versions and structure requirements**

Interaction-time decoding accepts verified versions and syntactically valid versions at or after `2.1.260` when every row retains a known structure.\
This forward-compatible boundary has no version ceiling.\
It does not expand the versions considered verified for `events` or `verify`.

**Time classification by record type**

Records affect the source time as follows:

| Record | Effect |
| --- | --- |
| Supported user or assistant message or tool row | Contributes its timestamp to the latest interaction time. |
| Recognized non-interaction attachment listed below | Does not advance the time. |
| Supported non-interaction system row, including `away_summary` and `informational` | Does not advance the time. |
| Bookkeeping row | Does not advance the time. |
| Provider API error | Does not advance the time. |

The following attachment types are recognized from their structure as non-interaction observations:

- `agent_listing_delta`
- `command_permissions`
- `deferred_tools_delta`
- `diagnostics`
- `edited_text_file`
- `opened_file_in_ide`
- `plan_mode`
- `plan_mode_exit`
- `selected_lines_in_ide`
- `skill_listing`
- `task_reminder`

**Conditions that prevent a source time**

The adapter does not return a source time in these cases:

- An attachment has another type, including one that might contain a queued command.\
  Its interaction meaning remains uncertain.
- A timestamp is missing or invalid.
- A potentially interactive row or block is unknown.
- A version is malformed, or is older than `2.1.260` and unverified.
- More than one transcript has the same session identity.

<a id="subagent-locations-and-sidecars"></a>

### Subagent record handling

**Transcript and sidecar locations**

Claude Code documents subagent transcripts at `projects/<project>/<session-id>/subagents/agent-<agent-id>.jsonl`.

The inspected distribution also writes adjacent metadata sidecars for these artifacts.

**File checks and reported problems**

The adapter recognizes only this contained path.\
It applies the same regular-file and root-containment checks and limits the size of sidecars that it decodes.\
It explicitly reports missing, orphaned, malformed, or oversized sidecars.

**Why subagents are not independent sources**

Subagent artifacts are not exposed as separate logical sources within the supported formats.\
The verified transcript and sidecar records do not provide an independent provider-owned subagent identity field.\
Deriving identity from a filename or relationships from a path would violate the common identity contract.

## Compatibility evidence

### Official location and lifecycle references

The following official documents describe record locations and lifecycle inputs:

- [Sessions](https://code.claude.com/docs/en/sessions) documents transcript persistence and the `projects/<project>/<session-id>.jsonl` location.
- [Environment variables](https://code.claude.com/docs/en/env-vars) documents `CLAUDE_CONFIG_DIR` and the default configuration directory behavior.
- [Hooks](https://code.claude.com/docs/en/hooks) documents the `session_id` and `transcript_path` lifecycle inputs.
- [Subagents](https://code.claude.com/docs/en/sub-agents) documents the subagent transcript location.

These references establish the location and lifecycle boundary.\
They do not make the internal JSONL entry structure a stable public API.

### Checks by version

Compatibility was checked using the following evidence:

| Version | Check |
| --- | --- |
| `2.1.177` | Read-only inspection of private provider-owned records, covered by independently constructed synthetic fixtures. Only structural properties were inspected; no private contents or identifiers were copied into this repository. |
| `2.1.228` | Official Claude Code distribution and the documented transcript, hook, status-line, and subagent location contracts. |
| `2.1.260` | Inspection of row-writing structures in the signed official distribution, without running it or using real transcripts as fixtures. |

These checks do not verify every possible row structure.\
Unknown rows and blocks still prevent `complete`.

### Read-only behavior checks

Synthetic integration tests compare the provider tree before and after representative `list`, `show`, `events`, and `verify` operations.\
They compare the file set, type and mode, size, modification time, and content hash.
