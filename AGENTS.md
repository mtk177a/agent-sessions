# AGENTS.md

This file defines repository-specific instructions for agents working on `agent-sessions`.

## Purpose

`agent-sessions` is a public, local-first tool for read-only access to coding-agent session records across providers.

The repository owns provider access, normalization, verification, and public contracts.

It does not own consumer-specific interpretation, retrospective findings, personal context, or processing state.

## Read first

Before making a material design or implementation change, read:

1. `README.md`
2. `docs/architecture.md`
3. any ADR directly relevant to the change

Do not scan unrelated documentation merely to increase coverage.

When provider behavior materially affects a design, verify the current provider documentation or implementation rather than relying on historical assumptions.

## Public information boundary

This is a public repository.

Do not add:

- private repository names or links;
- private Issue or Project references;
- personal or employer-specific context;
- real coding-agent session contents;
- credentials or authentication material;
- real usernames, hostnames, home directories, or machine-specific paths;
- private consumer implementation details.

Requirements learned from a private or environment-specific use case may be incorporated only after expressing them as general product requirements that stand on their own.

For example, it is valid to require multiple independently named Codex source instances. It is not valid to document the private system or machine that originally exposed that requirement.

## Core architecture constraints

Preserve the following unless an explicit architecture decision supersedes them:

- Provider-owned interaction records remain the raw source of truth.
- Read-only operations do not modify provider data or metadata.
- Read-only operations cause no persistent `agent-sessions` writes.
- The tool is semantically stateless by default.
- Consumer processing state does not belong in `agent-sessions`.
- Complete transcripts are not mirrored into an `agent-sessions` datastore.
- No daemon, watcher, timer, or periodic synchronization process is required.
- Multiple stores for the same provider remain distinct through named source instances.
- Filesystem paths are locators, not stable logical identities.
- Provider-internal storage layout belongs in provider adapters, not normal user configuration.
- Incomplete, unsupported, and failed observation remain distinguishable.

Do not introduce a database, persistent index, background process, hook-based ingestion path, or transcript mirror merely because it may become useful later.

Such additions require an observed need, measured evidence, and review of the relevant architecture decision.

## Disk and resource behavior

SSD write endurance is an explicit design concern.

Do not add persistent writes to a read-only command.

Do not add periodic whole-history scans or high-frequency state updates to improve interactive latency without measured evidence that the tradeoff is necessary.

Any future cache must be disposable and non-authoritative.

Deleting it must affect performance only.

When a change affects persistent I/O behavior, verify the write path directly rather than inferring safety from application-level payload size.

## Provider access

Prefer documented provider interfaces when they satisfy the repository's read-only contract.

Do not treat an interface as safe solely because it is official.

If an official read operation performs metadata repair, migration, synchronization, or another persistent mutation, evaluate a safer supported path.

Provider-specific parsing must remain isolated behind provider boundaries.

Do not expose unstable provider-internal structures as a stable public contract without an explicit compatibility decision.

## Fixtures and tests

Use synthetic fixtures only.

Synthetic fixtures should be minimal but must cover the behavior being tested, including malformed or adversarial input when relevant.

Do not derive committed fixtures by redacting real private sessions. Construct them independently.

Use obviously fictional values for credential-like test data.

## Consumer boundary

Do not add domain-specific concepts such as:

- whether a session has been reviewed;
- whether behavior was good or bad;
- retrospective findings or proposals;
- personal-memory classifications;
- downstream workflow status.

The access layer exposes source identity, source version, normalized observations, completeness, and verification evidence.

Consumers own the meaning applied to those observations.

## Language

Public repository artifacts are written in English.

This includes:

- code and identifiers;
- code comments;
- README and documentation;
- ADRs;
- CLI help and diagnostics;
- commit messages;
- release notes;
- repository policy files.

Do not maintain synchronized full Japanese translations of public artifacts.

Maintainer workflow may use Japanese when it reduces review or decision-making cost.

For maintainer-created public Issues and pull requests:

- use an English title;
- include a short English summary sufficient to identify the public change;
- detailed reasoning and review notes may be written in Japanese or English;
- do not duplicate the complete body as an English/Japanese translation pair.

External contributors are not required to follow the maintainer's Japanese workflow.

## Change discipline

Prefer the smallest change that fully satisfies the current requirement and preserves the public contracts.

Do not expand provider support, compatibility layers, caches, indexes, background processing, or configuration surfaces without a concrete requirement.

When changing an architectural invariant:

1. identify the affected decision;
2. determine whether the existing ADR still holds;
3. update or supersede the ADR when the decision changes;
4. keep `README.md`, architecture documentation, implementation, and tests consistent with the accepted state.

Do not add historical implementation narration to current-state documentation unless it is necessary to explain an active decision.

## Validation

Run the narrowest deterministic checks that can demonstrate the affected contract.

As implementation is added, document the canonical repository validation commands here or in a dedicated contributor document rather than inventing commands per task.

Always inspect the final diff for accidental private information and machine-specific paths before reporting completion.
