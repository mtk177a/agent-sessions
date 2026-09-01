# ADR 0001: Use stateless, read-only source access

- Status: Accepted
- Date: 2026-09-01

## Context

`agent-sessions` provides a common access layer over interaction records owned by coding-agent providers.

Different downstream consumers may inspect the same source for unrelated purposes.\
One consumer may have reviewed a source while another has not.\
There is therefore no single meaningful global "processed" state.\
Provider-owned interaction history already exists and can be inspected when needed.

Maintaining an additional authoritative session registry or transcript database would introduce synchronization, retention, privacy, and migration responsibilities that are not required to provide read access.

Continuous ingestion, periodic scans, and high-frequency local persistence would also consume resources when no consumer requires session data.\
Disk-write behavior is particularly important because local coding-agent tooling may process long-running, high-volume event streams, and unnecessarily frequent persistence can produce substantial write amplification.

## Decision

`agent-sessions` will use stateless, read-only source access by default.

The tool will:

- discover provider-owned sources when a command runs;
- read only the data required for the requested operation;
- normalize provider-specific records without taking ownership of them;
- return bounded machine-readable output;
- leave consumer processing state to downstream consumers.

The initial implementation will not require:

- a persistent session registry;
- SQLite or another application database;
- a local transcript mirror;
- a daemon;
- a file system watcher;
- periodic synchronization;
- lifecycle-hook ingestion;
- a persistent full-text index;
- a semantic or vector index.

The following requirements also apply:

- Read-only commands must cause no persistent `agent-sessions` writes.
- Previous execution of `agent-sessions` must not be required for correctness.
- Consumers that require incremental processing will retain their own reviewed source versions and compare them with current source metadata.
- When stronger provenance is required, `agent-sessions` may read the necessary source content and return a verified content hash or equivalent version identifier.
- A future disposable cache is permitted only when a measured performance problem justifies it.
- Such a cache must remain non-authoritative: removing it may reduce performance but must not alter a correct semantic result.

## Consequences

### Positive

- Provider-owned records remain the single source of truth for raw history.
- `agent-sessions` adds no idle processing or disk-write load.
- Consumer-specific processing semantics remain isolated.
- No initial inventory or database migration is required before using the tool.
- Stale application state cannot hide a source that still exists at the provider boundary.
- The architecture remains small and replaceable as provider capabilities evolve.
- Persistent SSD writes are avoided by default.

### Negative

- Source discovery work may be repeated across invocations.
- Large histories may make uncached listing or searching slower.
- A source removed by its provider cannot be reconstructed from `agent-sessions`.
- Full-history text search is more expensive without a persistent index.
- Consumers that require durable provenance must retain the minimum evidence they need.

These costs are accepted until realistic measurements demonstrate that they materially prevent an intended use case.

## Alternatives considered

### Maintain a shared persistent session registry

Rejected as the default architecture.\
It would create synchronization semantics and encourage unrelated consumer states to converge around one processing ledger.\
A downstream consumer may still maintain its own observation ledger when its domain requires historical source availability.

### Mirror provider transcripts into a local database

Rejected.\
Provider-owned records already contain the raw history.\
Duplicating them increases storage, privacy, synchronization, and migration costs without being necessary for the access layer.

### Ingest every session through lifecycle hooks

Rejected as the source-of-truth mechanism.\
Hooks may be missed, unavailable, disabled, or delayed and would introduce an additional stateful ingestion path.\
A hook may later be used as an optional optimization or notification mechanism without becoming authoritative.

### Periodically scan and index all history

Rejected.\
Background scanning would consume resources independently of an active consumer request and conflict with the zero-idle-activity goal.

## Revisit conditions

Revisit this decision when measured evidence demonstrates one or more of the following:

- direct discovery is materially too slow at realistic source volumes;
- provider interfaces cannot efficiently narrow candidate sources;
- repeated parsing creates unacceptable CPU or read-I/O cost;
- an intended consumer cannot implement incremental review without shared observation state;
- provider retention is too short for required provenance.

A revision should first consider a disposable metadata cache before introducing authoritative persistent state.
