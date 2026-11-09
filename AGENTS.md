# AGENTS.md

## Task references

- **Design changes**: Read [Architecture](docs/architecture.md) and the [ADRs](docs/architecture.md#architecture-decisions) related to the change.
- **Provider changes**: Obey the [rules for provider compatibility](docs/architecture.md#provider-compatibility-maintenance) and the document for the related provider.
- **Implementation validation**: Do the applicable [development checks](README.md#development-validation).

## Architecture constraints

Use the access layer for provider access, normalization, verification, and public contracts.\
Keep consumer-specific interpretation and processing state with consumers.

Keep these constraints unless an explicit architecture decision changes them:

- Keep provider-owned interaction records as the raw source of truth.
- During read-only operations, do not change provider data or metadata.\
  Do not write persistent `agent-sessions` data during these operations.
- Keep the tool semantically stateless by default.\
  Do not mirror complete transcripts into an `agent-sessions` datastore.
- A daemon, watcher, timer, or periodic synchronization process is not necessary.
- Use a different named source instance for each independent store of the same provider.\
  Use paths only as locators, not as stable logical identities.
- Keep knowledge of the provider's storage layout in its adapter, not in normal user configuration.
- Show which observations are incomplete, unsupported, or failed.

Keep provider-specific parsing inside its adapter.\
Without an explicit compatibility decision, do not make unstable provider internals part of a stable public contract.

Before you change an invariant, examine its ADR.\
If the decision changes, update or replace the ADR.\
Make sure that the documentation, implementation, and tests agree with the decision.

Without a requirement for the change, do not add provider support, compatibility layers, or configuration options.

### Read interface selection

When a documented provider interface has the necessary read-only behavior, use it as the first choice.\
Official read operations can still write data during metadata repair, migration, or synchronization.\
If an operation writes data, examine a safer supported path.

### Resource use

- Before you add a database, persistent index, background process, hook-based ingestion path, or transcript mirror, do these checks:
  - Make sure that observations and measurements show why the change is necessary.
  - Examine the related ADR.

- For interactive latency, do not add periodic whole-history scans or high-frequency state updates without measurements.\
  These measurements must show that the tradeoff is necessary.

- Keep any future cache disposable and non-authoritative.\
  If you delete the cache, only performance can change.

- When you change persistent I/O behavior, examine the write path.\
  Do not use application data size to show that the writes are safe.

## Repository data

### Public information

- Do not add real session contents, credentials, or data that identifies a real machine.\
  This includes real usernames, hostnames, home directories, and machine-specific paths.
- Do not add private repository names or links, or private Issue or Project references.\
  Do not add personal or employer-specific context, or private consumer implementation details.
- Before you use a requirement from a private or environment-specific case, write it as a general product requirement.\
  Do not include details of the original private system or machine.

### Synthetic test data

- Use only synthetic test data made independently of real private sessions.\
  Do not redact real private sessions to make this data.
- Make credential-like test values clearly fictional.

## Writing rules

Use English for the official versions of public repository artifacts and repository rules.\
For language choices and maintainer-created Issues and pull requests, use the [language policy](docs/localization.md).

- Use ASD-STE100 (Simplified Technical English) as a guide for English documentation.
- Write Japanese documentation in plain form (常体).
- When you add or change a maintained English file, use the [maintain-japanese-references Skill](.agents/skills/maintain-japanese-references/SKILL.md).
- For maintainer-created public Issues and pull requests, do not write the full body in both English and Japanese.
- If implementation history is not necessary to make an active design decision clear, do not include it in current-state documentation.
