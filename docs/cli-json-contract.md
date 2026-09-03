# CLI JSON contract

## Compatibility status

The initial schema is `v0alpha1`.\
It is intentionally provisional until production Codex and Claude Code adapters validate the common model.

Every response includes:

```json
{
  "schema_version": "v0alpha1",
  "cli_version": "dev",
  "redaction_policy_version": "v0alpha1",
  "operation": "list",
  "status": "complete",
  "data": {},
  "omissions": []
}
```

`data`, `page`, and `error` are present only when applicable.\
JSON is the only automation contract; human-readable output is not implemented.

## Operations

### `list`

`list` returns `data.sources` and a `page` object.\
Sources are sorted by logical source reference before offset pagination is applied.

### `show <source-ref>`

`show` returns one normalized source as `data.source`.

### `events <source-ref>`

`events` returns `data.source_ref`, ordered `data.events`, and a `page` object.\
Event indexes are zero-based and preserve provider order.

### `verify <source-ref>`

`verify` returns `data.source_ref` and `data.verified_version`.

The verified version is SHA-256 over a domain separator followed by name-sorted evidence chunks.\
Each chunk name and content is prefixed with its unsigned 64-bit big-endian byte length, and duplicate chunk names are rejected.

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

The raw provider-native ID is included in JSON only when final redaction can preserve it safely.\
The fingerprint and source reference remain available when the raw value is omitted or redacted.

## Events

Each event contains `index`, `kind`, typed event data, and bounded metadata.

Supported kinds are:

- `message`, with `role` and redacted `text`;
- `tool_call`, with a normalized `call_id` and safe operation `category`;
- `tool_result`, with the related `call_id`, `success`, and optional `exit_code`;
- `error`, with a safe `category` and redacted `message`.

Raw commands and raw tool arguments are not part of the public event model.

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

Pagination with `has_more: true` returns `partial`, an omission with code `pagination`, and a versioned `next_cursor`.\
Cursor contents are opaque to consumers.

## Structured errors and exit codes

Structured errors contain a stable `code`, `category`, redacted `message`, `retryable`, and bounded typed `details`.

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
  "schema_version": "v0alpha1",
  "sources": [
    {"id": "example-default", "provider": "example", "root": "/fictional/root"}
  ]
}
```

Provider and source instance IDs match `[a-z][a-z0-9-]{0,62}`.\
Source instance IDs are unique within one provider, roots are absolute, and unknown fields are rejected.

Resolution precedence is:

1. `--root` command-line override;
2. matching user configuration;
3. provider environment;
4. provider default.

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

## Final redaction

All dynamic strings pass through final redaction immediately before JSON encoding.\
This includes source metadata, version hints, messages, error events, omissions, structured errors, diagnostics, and build-version strings.

The initial policy removes credential-like values, authorization values, absolute Unix and Windows paths, UNC paths, file URIs, raw hostnames, and command-shaped text.\
Any material redaction adds an `output_redacted` omission and prevents a complete result.

Provider roots, raw commands, and raw tool arguments are excluded before this final pass.\
Safe operation category, success or failure, exit code, and call/result relationships remain structured evidence.
