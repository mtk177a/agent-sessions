# CLI JSON contract

Use this reference when writing scripts that call `agent-sessions` or interpret its JSON output.\
For installation and a first read, start with the [README](../README.md).\
Provider-specific roots, accepted formats, and input limits are documented for [Codex](provider-codex.md), [Claude Code](provider-claude-code.md), and [ChatGPT Data Export](provider-chatgpt-export.md).

## Calling commands

### Command syntax

Use these command forms.

```text
agent-sessions list [--config PATH] [--provider ID [--source-instance ID] [--root PATH]] [--repository PATH] [--limit N] [--cursor TOKEN]
agent-sessions show [--config PATH] [--root PATH] <source-ref>
agent-sessions events [--config PATH] [--root PATH] [--limit N] [--cursor TOKEN] <source-ref>
agent-sessions verify [--config PATH] [--root PATH] <source-ref>
```

### Options

The options apply to the commands shown in the table.

| Option | Applies to | Meaning |
| --- | --- | --- |
| `--config PATH` | All operations | Read the selected strict JSON configuration file. |
| `--provider ID` | `list` | Select one provider; absent means combine available providers. |
| `--source-instance ID` | `list`, with `--provider` | Select one named store or export boundary. |
| `--root PATH` | All operations; requires `--provider` for `list` | Override the provider-level root for this invocation. |
| `--repository PATH` | `list` | Select a local Git repository, including its linked worktrees; independent of the provider record root. |
| `--limit N` | `list`, `events` | Set page size; default 50, maximum 100. |
| `--cursor TOKEN` | `list`, `events` | Continue with the opaque cursor returned by the previous page. |

The `list --source-instance` and `list --root` options require `--provider` because source instance IDs and roots are provider-scoped.

### Selecting a source reference

Copy `<source-ref>` from `identity.source_ref` in a `list` response; do not construct it from a file path.\
Put flags before the positional source reference.

For a source listed from an explicit root, pass that root to subsequent source operations or configure the instance.\
A source reference does not contain the root's path.\
See [configuration](#configuration) for root selection and [Read every page](#read-every-page) for pagination.

### Find sessions for a local Git repository

```sh
agent-sessions list --repository /fictional/project
```

`PATH` selects an existing repository root, worktree, or subdirectory.\
Relative command-line paths are resolved from the invocation directory.\
Git must be available on `PATH` and support `rev-parse --path-format=absolute` and `worktree list --porcelain -z`.\
An unavailable target, Git executable, or target registration listing returns `invalid_repository` with exit code `2`.

The command compares normalized Git common administration directories.\
It does not merge independent clones by remote URL, branch, or commit, and it resolves nested repositories separately.\
A source matches when at least one recorded working directory resolves to the selected repository.\
Multiple matching observations produce one source; its events still include the entire supported conversation.

Filtering occurs before source-reference sorting and pagination.\
Keep the repository, record roots, configuration, and source snapshot unchanged between pages.\
`data.repository.common_dir` gives the resolved query target; it is a local locator, not a stable source identity.

Unresolved candidates are excluded from `data.sources` and counted in `repository_membership_unknown` omissions with scope `repository`.\
The optional token `reason` explains each group, and `count` counts candidates, not paths.\
A candidate with several gaps is counted once, with one of its reasons.\
These counts cover the full candidate set on every page and are separate from pagination omissions.\
A matching source with additional gaps remains included and produces `repository_evidence_incomplete`, also grouped by reason.\
Either omission makes the search `partial`, including the last page.\
An empty partial result does not establish that no matching conversation exists.

Reasons include `directory_unavailable`, `invalid_directory`, `git_unavailable`, `mapping_unavailable`, `conflicting_evidence`, `history_unavailable`, `ambiguous_artifact`, `unsupported_directory_record`, `invalid_directory_record`, `unsupported_provider`, `directory_limit`, and `git_lookup_limit`.\
Discovery, source-time, and other existing omissions remain in the response even when a source is filtered out.

## Configuration

### Configuration file

#### File selection

The default path is `agent-sessions/config.json` below the platform user configuration directory returned by `os.UserConfigDir`.\
An absent default file is valid and does not cause the directory or file to be created.

`--config` selects an explicit configuration file.\
It does not create or update that file.

#### Configuration structure

Configuration uses strict JSON with its own `schema_version: v1`.

#### Value constraints

Provider and source instance IDs match `[a-z][a-z0-9-]{0,62}`.\
Source instance IDs are unique within one provider, roots are absolute, and unknown fields are rejected.

#### Configuration example

The following example configures two independent Codex homes.

```json
{
  "schema_version": "v1",
  "sources": [
    {"id": "codex-one", "provider": "codex", "root": "/fictional/codex-one"},
    {"id": "codex-two", "provider": "codex", "root": "/fictional/codex-two"}
  ]
}
```

Replace each `root` with its actual absolute path.\
Choose stable `id` values: renaming an instance changes its source references.

Select one configured instance with `--source-instance`:

```sh
agent-sessions list --provider codex --source-instance codex-one
```

### Root resolution

#### Selection precedence

Resolution precedence is:

1. `--root` command-line override;
2. matching user configuration;
3. provider environment;
4. provider default.

#### Root access

Root selection and access have these rules:

| Condition | Behavior |
| --- | --- |
| A `list` root selected only through the provider environment or default does not exist. | The root contributes no source instance. |
| An explicit `--root` or configured root cannot be accessed. | The operation returns an error. |
| A provider has no environment or default root. | It contributes no implicit source instance. |

Provider-neutral discovery can therefore return available providers without requiring every registered provider to be installed.

#### Provider-specific root requirements

The `chatgpt` provider requires an explicit root.\
The root is the exact Data Export ZIP selected by the user, not a directory to search.

## JSON response structure

The public response schema is stable `v2`.\
The same contract is implemented by the production Codex, Claude Code, and ChatGPT Data Export adapters.

JSON is the only automation contract; human-readable output is not implemented.

### Response schema validation

A consumer must read and validate `schema_version` before interpreting `data` or any other version-dependent field.\
A consumer that implements this contract accepts `v2` and rejects an unknown major version.

After accepting the schema major, consumers must ignore unknown response fields.

### Common response fields

The following is the smallest complete `list` response:

```json
{
  "schema_version": "v2",
  "cli_version": "dev",
  "operation": "list",
  "status": "complete",
  "data": {
    "sources": []
  },
  "page": {
    "limit": 50,
    "has_more": false
  },
  "omissions": []
}
```

The common response fields have these presence rules:

| Field | Presence |
| --- | --- |
| `schema_version` | Required. Always `v2` for this contract. |
| `cli_version` | Required. Identifies the executable build and may be `dev`. |
| `operation` | Required. One of `list`, `show`, `events`, `verify`, or `cli` for command-level errors. |
| `status` | Required. One of the four [result states](#completeness). |
| `data` | Required for `complete` and `partial` operation results; absent for `unsupported` and `error`. |
| `page` | Required for successful or partial `list` and `events` results; absent otherwise. |
| `omissions` | Required. It is empty for `complete`, non-empty for `partial` and `unsupported`, and may record output processing omissions for `error`. |
| `error` | Required only for `error`; absent for every other status. |

### Command-specific data

The `data` object contains only the fields for the requested operation:

| Operation | Required `data` fields |
| --- | --- |
| `list` | `sources`, an array of source objects that may be empty |
| `show` | `source`, one source object |
| `events` | `source_ref` and `events`, an array that may be empty |
| `verify` | `source_ref` and `verified_version` |

#### `list`

`list` returns `data.sources` and a `page` object.\
Sources are sorted by logical source reference before offset pagination is applied.

#### `show <source-ref>`

`show` returns one normalized source as `data.source`.

#### `events <source-ref>`

`events` returns `data.source_ref`, ordered `data.events`, and a `page` object.\
Event indexes are zero-based and preserve provider order.

#### `verify <source-ref>`

`verify` returns `data.source_ref` and `data.verified_version`.

### Page information

**Fields**

A `page` object requires integer `limit` and Boolean `has_more`.\
`next_cursor` is required when `has_more` is true and absent otherwise.

**Cursor**

Cursor contents are opaque to consumers.

The current cursor begins with `v0:`.\
That cursor format version is independent from `schema_version`.

For the retrieval procedure, see [Read every page](#read-every-page).

See [status](#status) for the completeness of a page with more results.

## Returned data types

### Sources

A source is one addressable session or conversation.\
A source instance is its named provider store or export boundary.

#### Common fields

Each source requires `identity`, `kind`, `relationships`, and `metadata`.\
`working_directories`, `version_hint`, `last_interaction_at`, and `identity.provider_native_source_id` are optional; all other identity fields are required.

#### Logical identity

A source identity separates:

```text
provider
source instance
provider-native source ID
```

The command-line source reference has this form:

```text
as0:<provider>:<source-instance>:<source-id-fingerprint>
```

The fingerprint is a domain-separated SHA-256 hash of the provider-native source ID.\
The reference therefore remains deterministic without embedding the raw native ID or a file-system locator.

The `identity` object requires `provider`, `source_instance`, `provider_source_fingerprint`, and `source_ref`.\
Its optional `provider_native_source_id` contains the provider-owned logical ID within the output bound.\
The fingerprint and source reference remain available when the raw value is omitted.

The `as0` prefix and the source fingerprint's `v0` domain separator are versioned independently from `schema_version`.\
They do not change merely because the public response schema has reached `v2`.

#### Source kinds

Source `kind` is an extensible token; the currently emitted values are `session` for Codex and Claude Code records and `conversation` for ChatGPT Data Export conversations.\
Consumers must not reject an otherwise valid `v2` response solely because a source uses an unrecognized `kind` token.

#### Source relationships

Each relationship requires `kind` and `source_ref`.\
The currently emitted relationship kinds are `parent` and `forked_from`, both based only on explicit provider-owned identity fields.\
An adapter omits a relationship it cannot identify safely rather than deriving one from paths or event order.

#### Version hints

When present, `version_hint` requires `kind` and `value`, and its `kind` is also extensible.\
Codex sources without inherited history and Claude Code emit `stat_hash` for inexpensive change detection; Codex sources with safely resolved inherited history emit `content_hash` for the included rollout ranges.\
ChatGPT Data Export emits `snapshot_hash` for the SHA-256 identity of the complete export ZIP.\
Consumers may use a recognized hint for change detection and must ignore an unrecognized hint kind.

#### Source interaction time

`last_interaction_at`, when present, is a canonical UTC RFC 3339 timestamp ending in `Z`.\
It is the latest recorded user or assistant message, tool call, or tool result time in the logical source; management rows, system notifications, and file modification times are excluded.

**When the time is unavailable**

The field is absent if there is no interaction or the latest time cannot be established safely.\
Reasons include:

- Missing or malformed activity timestamps.
- Unknown rows that may contain interactions.
- Versions outside the provider's interaction-time compatibility boundary.
- Ambiguous artifacts.
- Unresolved inherited history.

Such absence adds `source_time_unavailable` with `scope: source` and makes `list` or `show` partial; `count` may aggregate affected sources.

**ChatGPT active branch**

For ChatGPT Data Export, the selected active branch defines the interactions considered; other branches do not advance this field.

#### Recorded working directories

`list` and `show` can return optional `working_directories`.\
Absence means that this observation is unsupported, not that the source worked in no repository.\
This object has required `state`, `locations`, and `reasons` fields; both arrays can be empty.\
`state` is `complete`, `partial`, or `unsupported` and describes extraction of recorded directory evidence.\
`partial` and `unsupported` require token reasons; `complete` has no reasons.\
A complete extraction does not prove that every actual directory change was recorded.

Each location has these fields:

| Field | Meaning |
| --- | --- |
| `path` | The recorded absolute directory after provider-specific path decoding, without truncation or inference from text. |
| `evidence` | `session`, `turn`, `tool`, `interaction`, or `worktree`: the normalized kind of recorded evidence. |
| `mapping` (optional) | Recorded worktree relationship: required `original_path` and `kind`, and optional `previous_path`. The kind is `git_created`, `existing`, `hook`, or `unknown`. |
| `repository` (optional) | Local resolution: required `state`, either `resolved` or `unknown`. |

A resolved repository requires `common_dir` and `basis`, with no `reason`.\
The basis is `current_git`, `git_registration`, or `provider_mapping`.\
An unknown repository requires a token `reason` and has no `common_dir` or `basis`.\
Provider results can omit resolution; CLI `list` and `show` resolve the returned locations.

Current Git resolution uses the nearest repository and resolves symbolic links in administration paths.\
It is evidence about the current filesystem, not proof of historical membership.\
Moves, undetected path reuse, and records copied from another environment cannot be reconstructed from paths alone.\
When available evidence conflicts, the location remains unknown; the CLI does not choose one association.\
A conflict at one path invalidates other associations at that same path within the source, but does not discard independent matching locations.

For an absent worktree root, a remaining registration in the query repository can establish membership.\
This fallback requires the exact root path; an absent descendant could have contained a separate nested repository.\
`show` has no query repository and therefore does not search its registrations.\
A verified provider mapping for normal Git creation can resolve an absent root from its original directory, provided that any recorded previous directory also resolves to the same repository.\
If the original or previous directory cannot be resolved, membership established by current Git evidence or a remaining registration still applies.\
Existing-worktree entry and Hook creation never establish that relationship by themselves.\
An existing path with conflicting or unusable administration is not recovered as a deleted worktree.\
Unrecoverable locations stay unknown.

There are at most 256 distinct directory observations per source and 4,096 uncached Git directory lookups per invocation.\
Further observations or lookups remain incomplete.\
Each Git read has a five-second timeout and a 1 MiB stdout limit.\
Paths and structural fields are never shortened into different identities.

Without `--repository`, the envelope status retains its existing source-metadata meaning.\
Examine `working_directories.state`, `reasons`, and each `repository.state` for this optional observation.\
With the filter, uncertainty also affects the envelope as specified in [repository search](#find-sessions-for-a-local-git-repository).\
Directory evidence does not change source references, hints, event indexes, or verified provider-content versions.\
Git metadata used for resolution is not part of `verify` evidence.\
Directory paths and Git administration paths can disclose private information and are not redacted.

### Events

#### Common fields

Each event requires `index`, `kind`, exactly one kind-specific data object, and bounded `metadata`.

#### Recorded event time

Events report `time_state` and, when established, `recorded_at` in canonical UTC RFC 3339.\
The timestamp belongs to the provider record, not an inferred operation start or end.\
`time_state` is `available`, `absent`, `unavailable`, or `unsupported`; only `available` includes `recorded_at`.\
Missing or unusable times add `event_time_omitted` and make `events` partial.

#### Kind-specific data

Each event has one typed data object for its `kind`:

##### Message

`message`: `role` (`user` or `assistant`), recorded `text`, and Boolean `truncated`.

##### Tool call

`tool_call`: optional `call_id` and recorded `name`, required `category` and `input_state`, optional `action` and `input`.

**Tool name**

Tool names are opaque provider-specific values, not standardized tool interfaces.\
A missing recorded name is omitted and reported as `tool_name_unavailable`.

**Operation classification**

`category` retains broad classifications, and `action`, when present, is `execute`, `read`, `search`, `write`, `edit`, or `invoke`.

**Input**

For `input_state`, see [Content acquisition states](#content-acquisition-states).\
For the `input` representation, see [Content objects](#content-objects).

##### Tool result

`tool_result`: required `outcome`, `correlation_state`, and `content_state`, optional `call_id`, `exit_code`, and `content`.

**Outcome**

`outcome` is `success`, `failure`, or `unknown`.\
Only explicit, verified provider fields establish success or failure; neither `PASS` nor error-like output text does so.\
`unknown` retains readable content and adds `tool_outcome_unknown`.

**Content**

For `content_state`, see [Content acquisition states](#content-acquisition-states).\
For the `content` representation, see [Content objects](#content-objects).

##### Provider error

`error`: `category` and the recorded provider error `message`.

#### Call/result relationships

**Correlation states**

`correlation_state` uses these values:

| State | Condition | Returned result |
| --- | --- | --- |
| `matched` | One unique call and result have an explicit correlation identifier in the selected logical history. | Includes `call_id`. |
| `unmatched` | The matching call cannot be established. | Retains the body, omits `call_id`, and adds `correlation_omitted`. |
| `ambiguous` | Duplicate call or result identifiers prevent unique correspondence. | Retains the body, omits `call_id`, and adds `correlation_omitted`. |

Only `matched` includes result `call_id`.\
A result may appear before its uniquely identified call, or on a different page.\
Calls with missing or duplicated identifiers are retained without `call_id`; calls without a recorded result also report an omission.\
No relationship is inferred from adjacency or matching text.

**History used to resolve relationships**

Correlation is resolved over the entire selected history before pagination.\
Normalized call IDs retain their adapter-owned `v0` algorithm namespace, independent from the response schema; raw provider correlation identifiers are not public call IDs.

### Content objects

`input` and `content` use this shared representation:

```json
{"format":"json","text":"[\"go\",\"test\",\"./...\"]","truncated":false}
```

**Fields**

The content object has these fields:

| Field | Rule |
| --- | --- |
| `format` | `text` or `json`. |
| `text` | Always a string. |
| `truncated` | Required Boolean. |
| `omitted` | Optional Boolean. Reports excluded non-text or unsupported portions when readable content remains. |

**JSON preservation**

A JSON body is valid JSON unless `truncated` is true; a truncated prefix may not parse.\
JSON formatting and object key order are not byte-preservation guarantees, but string values and array order are retained within the bounds.\
An argument array remains an array encoded in the JSON string; it is never joined into an executable command.

Keys inside recorded JSON inputs or results are opaque provider-specific values, not standardized tool interfaces.

### Content acquisition states

`input_state` and `content_state` use these values:

| State | Meaning and content object |
| --- | --- |
| `available` | The corresponding content object is required, including for an empty string, object, or array. |
| `absent` | The verified format establishes absence. The content object is omitted. |
| `unavailable` | Expected information is missing or unreadable. The content object is omitted. |
| `unsupported` | The content shape is not supported. The content object is omitted. |

`unavailable` and `unsupported` add `tool_input_unavailable` or `tool_content_unavailable` omissions.

For body limits and truncation, see [Individual content bodies](#individual-content-bodies).

### Verified versions

Use this content version for provenance or to compare observations of the same source.\
Verification does not mark the source as reviewed or retain a transcript.

#### Fields

`verified_version` requires `algorithm`, `basis`, and `value`.\
The current values use `sha256`, `provider-content-v0`, and a `sha256:`-prefixed digest.

#### Verified-version encoding

The verified version is SHA-256 over a domain separator followed by name-sorted evidence chunks.\
Each chunk name and content is prefixed with its unsigned 64-bit big-endian byte length, and duplicate chunk names are rejected.

The hash uses its own `v0` domain separator and `provider-content-v0` basis, independently from `schema_version`.\
They change only when the hashing meaning or algorithm changes.

### Metadata

**Entry format**

Each metadata entry requires token-valued `name` and a safe string `value`.\
Metadata names do not grant provider-specific values a provider-neutral meaning.

**Conditions for exposing provider information**

Provider-specific artifact layouts, row types and correlation values remain inside the owning adapter.\
Only bounded metadata that is safe to expose without inventing a provider-neutral meaning may appear in the common `metadata` arrays.

An adapter reports a material value that cannot be represented faithfully as an omission.\
It does not promote an unstable provider field into the stable schema or claim `complete` by guessing an equivalent common meaning.

## Interpreting results

### Completeness

#### Status

The response reports one of these states in `status`:

| Status | Meaning |
| --- | --- |
| `complete` | No material value was omitted, truncated, unsupported, or unknown. |
| `partial` | Useful output exists, but at least one omission is reported. |
| `unsupported` | The provider or operation cannot safely satisfy the contract. |
| `error` | The operation failed and includes a structured error. |

A `complete` response cannot contain omissions or an error.\
A `partial` response contains at least one omission.\
Unknown provider shapes and values whose meaning cannot be established must not produce `complete`.

**Pagination status**

Pagination with `has_more: true` returns `partial`, an omission with code `pagination`, and a versioned `next_cursor`.

#### Omissions

Each omission can include an optional token-valued `reason`.\
Every omission requires token-valued `code` and `scope` plus a safe human-readable `message`.\
The optional positive integer `count` reports how many known items that omission represents; its absence does not imply zero affected provider values.

#### Combining results from multiple providers

For `list`, multiple adapter results are aggregated independently of adapter order.\
If every observed adapter result is unsupported, the response is `unsupported`; if an unsupported omission is combined with any complete or partial observation, the response is `partial`.

### Errors

#### Error object

Structured errors require token-valued `code` and `category`, a fixed human-readable `message`, Boolean `retryable`, and a `details` array.\
Each detail entry requires token-valued `name` and a safe string `value`; the array may be empty.\
The current CLI errors are not retryable.

#### Error categories

The stable error categories are `usage`, `configuration`, `provider`, `resource`, and `internal`.

#### Error codes

The current error codes are:

- `missing_command`.
- `unknown_command`.
- `invalid_arguments`.
- `invalid_pagination`.
- `invalid_cursor`.
- `invalid_source_ref`.
- `invalid_repository`.
- `source_resolution_failed`.
- `config_path_unavailable`.
- `invalid_configuration`.
- `source_not_found`.
- `provider_failure`.
- `invalid_provider_result`.
- `invalid_archive`.
- `invalid_json`.
- `duplicate_archive_member`.
- `unsafe_archive_member`.
- `source_changed`.
- `provider_resource_limit`.
- `event_limit_exceeded`.
- `verification_failed`.
- `output_bound_exceeded`.
- `response_limit_exceeded`.
- `invalid_result`.

Omission and error code sets may grow within `v2`.\
After accepting `schema_version`, consumers must preserve the declared incomplete or error state when they encounter an unknown code instead of treating the result as complete.

### Exit codes

The CLI uses these exit codes:

| Exit code | Meaning |
| --- | --- |
| `0` | `complete` or `partial` result |
| `2` | Invalid command, flag, cursor, source reference, or configuration |
| `3` | Unsupported provider or operation |
| `4` | Provider, I/O, verification, resource, encoding, or internal failure |

## Resource bounds

### Bound summary

The common limits are:

| Resource | Bound |
| --- | ---: |
| Configuration file | 1 MiB |
| Buffered provider or verification evidence | 64 MiB |
| JSON nesting | 64 levels |
| Events per source observation | 100,000 |
| Dynamic output string | 64 KiB |
| One message, tool input, or tool result content | 64 KiB |
| Metadata entries per source or event | 64 |
| Relationships per source | 64 |
| Page size | default 50, maximum 100 |
| Encoded response | 8 MiB |

### Input bound scope

The common 64 MiB evidence bound applies to adapters that return evidence chunks to the CLI.\
An archive adapter may stream a larger, provider-bounded source into the same `provider-content-v0` verified-version algorithm without materializing it as an evidence chunk.\
Provider-specific input bounds are documented with each adapter and do not increase the 8 MiB response bound.

### Output that exceeds a limit

#### String fields

Truncated dynamic output adds a `resource_truncation` omission and cannot remain complete.

Free-form strings are truncated only at a valid UTF-8 boundary.\
An oversized raw provider-native source ID is omitted while its fingerprint and source reference remain available.\
Oversized structural identifiers fail closed with a structured resource error instead of being truncated into a different identity.

#### Individual content bodies

The entire content object body is bounded to 64 KiB, including when several result fields or text blocks are represented together.\
Content truncation sets `truncated` and reports `tool_input_truncated` or `tool_content_truncated`, making the response partial.\
Truncation performed by the common final output stage reports `resource_truncation`.\
Excluded non-text or unsupported portions report omissions such as `unsupported_content`.\
There is no continuation operation for one truncated body.

Reducing the page limit does not recover an individually truncated body.

#### Whole response

If JSON encoding exceeds 8 MiB, the response is `response_limit_exceeded`, never an incomplete JSON stream.\
For paginated operations, retry from the same cursor with a smaller `--limit`.\
`retryable` remains false because repeating the unchanged request does not resolve the limit.\
Reducing the page limit does not recover an individually truncated body.

## Recorded content handling

### Content disclosure policy

Recorded content is returned without automatic redaction, including credential-like values, paths, hostnames, code, search text, and command text.\
There is no redaction switch and no `redaction_policy_version` output field.\
Consumers decide what to analyze, retain, or share externally.

### Handling untrusted data

Historical content remains untrusted data and is never executed or treated as current instructions.

### Information in diagnostics

CLI diagnostics do not unnecessarily copy underlying errors or source records.

Underlying errors, source roots, and raw arguments are not copied into public diagnostics.

## Consumer procedures

### Read every page

For `list` and `events`, keep the same source selection for every request, including any explicit root or configuration.

Follow these steps to read the pages:

1. Validate `schema_version` before interpreting version-dependent fields.
2. Inspect `status` and `omissions`, and read any available operation data.
3. If `page.has_more` is `true`, pass `page.next_cursor` unchanged to the same operation.
4. Continue until `has_more` is `false`, retaining omissions from all pages.

Use the returned cursor in these command forms:

```sh
agent-sessions list --provider codex --limit 50 --cursor '<page.next_cursor>'
agent-sessions events --limit 50 --cursor '<page.next_cursor>' '<source_ref>'
```

**Completeness after the last page**

Finishing pagination does not resolve other omissions, and exit code `0` does not establish completeness.\
A smaller page limit does not recover an individually truncated body; see [Individual content bodies](#individual-content-bodies).

**Source changes between requests**

Offset pagination cannot guarantee a fixed snapshot if the source changes between requests.\
Use the same source data and executable, and compare verified versions before and after when required.

### Selecting recent sources

For a fixed lookback period, select sources as follows:

1. Collect every `list` page.
2. Exclude sources without `last_interaction_at` from the subset with known times.
3. Compare timestamps as absolute instants for the requested period.
4. Sort eligible sources by `last_interaction_at` descending, then by `identity.source_ref` ascending.

A partial listing, including a missing source time or an unfinished page, cannot establish that the selected sources are exhaustive.

### Updating a v1 consumer

See [Changes from v1](#changes-from-v1) for changed fields and content handling.

Before updating the pinned executable, complete these steps:

1. Make the consumer accept v2 and read input and result content.
2. Handle unknown values and omissions.
3. Review retention and sharing.

**Previous executable**

Keep or restore the previous executable when v1 is required or integration fails.

## Versioning

### Response schema change rules

Additive optional fields may be introduced within `v2`.\
Removing or changing a required field, changing an existing field's meaning, or incompatibly changing a status, event kind, exit code, logical identity rule, or other required semantic requires a new major schema version.

### Independent format versions

`cli_version` reports the executable version and does not select the response schema.\
The configuration schema is separately validated and currently accepts exactly `v1`.

Source references, fingerprint hashing, verification hashing, call IDs, and cursors also have versions independent from the response schema.\
Their formats are defined in [Logical identity](#logical-identity), [Verified-version encoding](#verified-version-encoding), [Call/result relationships](#callresult-relationships), and [Page information](#page-information).

### Changes from v1

**Changed behavior**

v2 removes automatic redaction guarantees and represents inputs, results, unknown outcomes, and unresolved correlations.\
`success`, `excerpt`, `redacted`, and `evidence_state` are replaced by the new fields; `input_state: withheld` becomes `available` with actual `input`.\
The new executable has no v1 output mode.

**Unchanged formats**

Configuration `schema_version: v1`, commands and arguments, source references, and verification hashing are unchanged.
