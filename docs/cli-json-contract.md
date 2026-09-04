# CLI JSON contract

## Compatibility status

The public response schema is stable `v1`.\
The same contract is implemented by the production Codex and Claude Code adapters.

The following is the smallest complete `list` response:

```json
{
  "schema_version": "v1",
  "cli_version": "dev",
  "redaction_policy_version": "v1",
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

JSON is the only automation contract; human-readable output is not implemented.

The envelope fields have these presence rules:

| Field | Presence |
| --- | --- |
| `schema_version` | Required. Always `v1` for this contract. |
| `cli_version` | Required. Identifies the executable build and may be `dev`. |
| `redaction_policy_version` | Required. Always `v1` for the current policy. |
| `operation` | Required. One of `list`, `show`, `events`, `verify`, or `cli` for command-level errors. |
| `status` | Required. One of the four completeness states below. |
| `data` | Required for `complete` and `partial` operation results; absent for `unsupported` and `error`. |
| `page` | Required for successful or partial `list` and `events` results; absent otherwise. |
| `omissions` | Required. It is empty for `complete`, non-empty for `partial` and `unsupported`, and may record output processing omissions for `error`. |
| `error` | Required only for `error`; absent for every other status. |

### Compatibility rule

A consumer must read and validate `schema_version` before interpreting `data` or any other version-dependent field.\
A consumer that implements this contract accepts `v1` and rejects an unknown major version.

Additive optional fields may be introduced within `v1`, and consumers must ignore unknown response fields after accepting the schema major.\
Removing or changing a required field, changing an existing field's meaning, or incompatibly changing a status, event kind, exit code, logical identity rule, or other required semantic requires a new major schema version.

`cli_version` reports the executable version and does not select the response schema.\
The configuration schema is separately validated and currently accepts exactly `v1`.

## Operations

### `list`

`list` returns `data.sources` and a `page` object.\
Sources are sorted by logical source reference before offset pagination is applied.

Each source requires `identity`, `kind`, `relationships`, and `metadata`.\
`version_hint` and `identity.provider_native_source_id` are optional; all other identity fields are required.

### `show <source-ref>`

`show` returns one normalized source as `data.source`.

### `events <source-ref>`

`events` returns `data.source_ref`, ordered `data.events`, and a `page` object.\
Event indexes are zero-based and preserve provider order.

### `verify <source-ref>`

`verify` returns `data.source_ref` and `data.verified_version`.

The verified version is SHA-256 over a domain separator followed by name-sorted evidence chunks.\
Each chunk name and content is prefixed with its unsigned 64-bit big-endian byte length, and duplicate chunk names are rejected.

`verified_version` requires `algorithm`, `basis`, and `value`.\
The current values use `sha256`, `provider-content-v0`, and a `sha256:`-prefixed digest.

### Operation data and page objects

The `data` object contains only the fields for the requested operation:

| Operation | Required `data` fields |
| --- | --- |
| `list` | `sources`, an array of source objects that may be empty |
| `show` | `source`, one source object |
| `events` | `source_ref` and `events`, an array that may be empty |
| `verify` | `source_ref` and `verified_version` |

A `page` object requires integer `limit` and Boolean `has_more`.\
`next_cursor` is required when `has_more` is true and absent otherwise.

Every omission requires token-valued `code` and `scope` plus a safe human-readable `message`.\
The optional positive integer `count` reports how many known items that omission represents; its absence does not imply zero affected provider values.

Each metadata entry requires token-valued `name` and a safe string `value`.\
Metadata names do not grant provider-specific values a provider-neutral meaning.

Each relationship requires `kind` and `source_ref`.\
The currently emitted relationship kinds are `parent` and `forked_from`, both based only on explicit provider-owned identity fields.\
An adapter omits a relationship it cannot identify safely rather than deriving one from paths or event order.

A source has `kind: "session"`.\
When present, `version_hint` requires `kind` and `value`; the current adapters emit a `stat_hash` hint for inexpensive change detection.

## Logical identity

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
Its optional `provider_native_source_id` contains the provider-owned logical ID when final redaction can preserve it safely.\
The fingerprint and source reference remain available when the raw value is omitted or redacted.

The `as0` prefix and the source fingerprint's `v0` domain separator are versioned independently from `schema_version`.\
They do not change merely because the public response schema has reached `v1`.

## Events

Each event requires `index`, `kind`, exactly one typed event payload, and bounded `metadata`.

Supported kinds are:

- `message`, with `role` and redacted `text`;
- `tool_call`, with a normalized `call_id` and safe operation `category`;
- `tool_result`, with the related `call_id`, `success`, and optional `exit_code`;
- `error`, with a safe `category` and redacted `message`.

A tool result must reference one earlier unique tool call, and at most one normalized result may reference a call.\
Provider adapters report duplicate, missing, or unmatched correlation as an omission instead of emitting an ambiguous event sequence.

Raw commands and raw tool arguments are not part of the public event model.

Normalized call IDs use an adapter-owned `v0` algorithm namespace independent from `schema_version`.\
Provider correlation identifiers are not public call IDs.

## Provider-specific information

Provider-specific artifact layouts, row types, raw tool names, and correlation values remain inside the owning adapter.\
Only bounded metadata that is safe to expose without inventing a provider-neutral meaning may appear in the common `metadata` arrays.

An adapter reports a material value that cannot be represented faithfully as an omission.\
It does not promote an unstable provider field into the stable schema or claim `complete` by guessing an equivalent common meaning.

## Completeness and omissions

| Status | Meaning |
| --- | --- |
| `complete` | No material value was omitted, truncated, unsupported, unknown, or unsafely redacted. |
| `partial` | Useful output exists, but at least one omission is reported. |
| `unsupported` | The provider or operation cannot safely satisfy the contract. |
| `error` | The operation failed and includes a structured error. |

A `complete` response cannot contain omissions or an error.\
A `partial` response contains at least one omission.\
Unknown provider shapes and values whose output safety cannot be established must not produce `complete`.

For `list`, multiple adapter results are aggregated independently of adapter order.\
If every observed adapter result is unsupported, the response is `unsupported`; if an unsupported omission is combined with any complete or partial observation, the response is `partial`.

Pagination with `has_more: true` returns `partial`, an omission with code `pagination`, and a versioned `next_cursor`.\
Cursor contents are opaque to consumers.

The current cursor begins with `v0:`.\
That cursor format version is independent from `schema_version`.

## Structured errors and exit codes

Structured errors require token-valued `code` and `category`, a redacted human-readable `message`, Boolean `retryable`, and a `details` array.\
Each detail entry requires token-valued `name` and a safe string `value`; the array may be empty.\
The current CLI errors are not retryable.

The stable error categories are `usage`, `configuration`, `provider`, `resource`, and `internal`.\
The current error codes are:

- `missing_command`, `unknown_command`, `invalid_arguments`, `invalid_pagination`, `invalid_cursor`, and `invalid_source_ref`;
- `source_resolution_failed`, `config_path_unavailable`, and `invalid_configuration`;
- `source_not_found`, `provider_failure`, and `invalid_provider_result`;
- `event_limit_exceeded`, `verification_failed`, `output_bound_exceeded`, and `response_limit_exceeded`;
- `invalid_result`.

Omission and error code sets may grow within `v1`.\
After accepting `schema_version`, consumers must preserve the declared incomplete or error state when they encounter an unknown code instead of treating the result as complete.

| Exit code | Meaning |
| --- | --- |
| `0` | `complete` or `partial` result |
| `2` | Invalid command, flag, cursor, source reference, or configuration |
| `3` | Unsupported provider or operation |
| `4` | Provider, I/O, verification, resource, encoding, or internal failure |

Underlying errors, source roots, and raw arguments are not copied into public diagnostics.

## Configuration

The default path is `agent-sessions/config.json` below the platform user configuration directory returned by `os.UserConfigDir`.\
An absent default file is valid and does not cause the directory or file to be created.

Configuration is strict JSON:

```json
{
  "schema_version": "v1",
  "sources": [
    {"id": "example-default", "provider": "example", "root": "/fictional/root"}
  ]
}
```

Provider and source instance IDs match `[a-z][a-z0-9-]{0,62}`.\
Source instance IDs are unique within one provider, roots are absolute, and unknown fields are rejected.

The `list --source-instance` and `list --root` options require `--provider` because source instance IDs and roots are provider-scoped.

Resolution precedence is:

1. `--root` command-line override;
2. matching user configuration;
3. provider environment;
4. provider default.

For `list`, an absent root selected only through provider environment or provider default contributes no source instance.\
This allows provider-neutral discovery to return available providers without requiring every registered provider to be installed.\
An explicit `--root` or configured root is authoritative, so an access failure for either remains an operation error.

`--config` selects an explicit configuration file.\
It does not create or update that file.

## Resource bounds

| Resource | Bound |
| --- | ---: |
| Configuration file | 1 MiB |
| Provider or verification evidence | 64 MiB |
| JSON nesting | 64 levels |
| Events per source observation | 100,000 |
| Dynamic output string | 64 KiB |
| Metadata entries per source or event | 64 |
| Relationships per source | 64 |
| Page size | default 50, maximum 100 |
| Encoded response | 8 MiB |

Truncated dynamic output adds a `resource_truncation` omission and cannot remain complete.

Free-form strings are truncated only at a valid UTF-8 boundary.\
An oversized raw provider-native source ID is omitted while its fingerprint and source reference remain available.\
Oversized structural identifiers fail closed with a structured resource error instead of being truncated into a different identity.

## Final redaction

All dynamic strings pass through final redaction immediately before JSON encoding.\
This includes source metadata, version hints, messages, error events, omissions, structured errors, diagnostics, and build-version strings.

The `v1` policy removes credential-like values, authorization values, absolute Unix and Windows paths, UNC paths, file URIs, raw hostnames, and command-shaped text.\
Any material redaction adds an `output_redacted` omission and prevents a complete result.

Provider roots, raw commands, and raw tool arguments are excluded before this final pass.\
Safe operation category, success or failure, exit code, and call/result relationships remain structured evidence.
