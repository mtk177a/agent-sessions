# ADR 0004: Return bounded recorded content without automatic redaction

- Status: Accepted
- Date: 2026-09-27

## Context

Consumers need recorded commands, tool inputs, ordinary output, structured results, and errors to inspect what an agent actually did.
The v1 contract withholds inputs, filters results to a narrow line grammar, and redacts credential-like values, paths, hosts, and command text.
These transformations remove evidence needed for the current inspection use case.
Unknown outcomes and unresolved correlations also cause readable result bodies to be dropped.

## Decision

The v2 output contract returns supported recorded content without automatic redaction or a redaction mode switch.
Credentials and private paths may therefore appear in output.
Consumers own selection for analysis, retention, and external sharing.
Public fixtures and repository artifacts continue to exclude real credentials and private records.
CLI diagnostics use controlled messages rather than unnecessarily copying underlying errors or source data.

Inputs and result bodies use bounded text or JSON-string content, retaining argument arrays and distinct result fields.
Each body is limited to 64 KiB, with explicit UTF-8 prefix truncation and omissions.
The encoded response remains limited to 8 MiB, with explicit errors when a requested page exceeds the limit.
Unknown outcomes and unresolved or ambiguous correlations retain readable bodies and reduce completeness.
Only verified explicit provider fields establish outcomes and relationships; result text and adjacency never do.

v2 replaces v1 rather than silently weakening its guarantees.
There is no v1 mode in the new executable; consumers may keep or restore an older pinned executable.
Configuration, source identity, verification hashing, provider-version acceptance, and source selection remain independently versioned and unchanged.

ADR 0001 and ADR 0003 remain valid.
Provider-owned records stay authoritative; reads remain non-mutating and cause no persistent agent-sessions writes.
Historical content remains untrusted data and is never executed or treated as current instructions.
Provider-specific decoding stays in adapters; unknown shapes remain incomplete rather than becoming a raw-file passthrough.

## Consequences

Consumers receive substantially more useful evidence and must explicitly accept the content exposure before upgrading.
The access layer does not attempt to decide which recorded values a consumer should share or retain.
Size limits and omission states remain necessary: v2 is a bounded observation contract, not a complete transcript archive.
A single truncated body has no continuation operation.
Updating consumers requires v2 schema acceptance, new input/result fields, and unknown/omission handling.

## References

- [CLI JSON contract](../cli-json-contract.md)
- [Architecture](../architecture.md)
- [Issue #23](https://github.com/mtk177a/agent-sessions/issues/23)
