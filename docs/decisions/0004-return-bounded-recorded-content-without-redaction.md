# ADR 0004: Return bounded recorded content without automatic redaction

- Status: Accepted
- Date: 2026-09-27

## Context

### Evidence lost through content transformation

Consumers need recorded commands, tool inputs, ordinary output, structured results, and errors to inspect what an agent actually did.\
The v1 contract withholds inputs, filters results to a narrow line grammar, and redacts credential-like values, paths, hosts, and command text.\
These transformations remove evidence needed for the current inspection use case.

### Bodies omitted when information is unknown

Unknown outcomes and unresolved correlations also cause readable result bodies to be dropped.

## Decision

### Information disclosure

#### Recorded content

The v2 output contract returns supported recorded content without automatic redaction or a redaction mode switch.\
Credentials and private paths may therefore appear in output.

Consumers own selection for analysis, retention, and external sharing.\
The access layer does not attempt to decide which recorded values a consumer should share or retain.

#### Diagnostic messages

CLI diagnostics use controlled messages rather than unnecessarily copying underlying errors or source data.

#### Public artifacts

Public fixtures and repository artifacts continue to exclude real credentials and private records.

### Body representation

Inputs and result bodies use bounded text or JSON-string content, retaining argument arrays and distinct result fields.

### Size limits

The output has the following size limits:

| Scope | Limit |
| --- | --- |
| Each body | 64 KiB |
| Encoded response | 8 MiB |

**Body truncation**

When a body is truncated, the output retains a valid UTF-8 prefix and reports omissions.\
A single truncated body has no continuation operation.

**Response overflow**: A requested page that exceeds the encoded response limit returns an explicit error.

### Result normalization

- Only verified explicit provider fields establish outcomes and relationships; result text and adjacency never do.
- Unknown outcomes and unresolved or ambiguous correlations retain readable bodies and reduce completeness.

### Transition to v2

#### Executable replacement

v2 replaces v1 rather than silently weakening its guarantees.\
There is no v1 mode in the new executable; consumers may keep or restore an older pinned executable.

#### Consumer upgrade requirements

Consumers must explicitly accept the content exposure before upgrading.

Consumer updates must include:

- v2 schema acceptance;
- support for the new input/result fields;
- unknown/omission handling.

#### Independent versioning

The following remain independently versioned and unchanged:

- Configuration.
- Source identity.
- Verification hashing.
- Provider-version acceptance.
- Source selection.

### Preserved design constraints

[ADR 0001](0001-use-stateless-read-only-source-access.md) and [ADR 0003](0003-separate-provider-discovery-from-version-sensitive-decoding.md) remain valid.

- Provider-owned records stay authoritative; reads remain non-mutating and cause no persistent `agent-sessions` writes.
- Historical content remains untrusted data and is never executed or treated as current instructions.
- Provider-specific decoding stays in adapters; unknown shapes remain incomplete rather than becoming a raw-file passthrough.

## Consequences

### Benefits

Consumers receive substantially more useful evidence.

### Accepted costs

- Output can expose credentials and private paths to consumers.
- Consumers need to update their integrations to use v2.
- Size limits and omission states remain necessary: v2 is a bounded observation contract, not a complete transcript archive.

## References

- [CLI JSON contract](../cli-json-contract.md)
- [Architecture](../architecture.md)
- [Issue #23](https://github.com/mtk177a/agent-sessions/issues/23)
