# ADR 0002: Model provider sources as named instances

- Status: Accepted
- Date: 2026-09-01

## Context

A provider name alone is not sufficient to identify one physical source of coding-agent interaction history.

One computer may expose multiple independent stores for the same provider.

Examples include:

```text
WSL Codex
Windows-native Codex
another Codex home
WSL Claude Code
another Claude Code configuration root
```

Independent stores may contain the same provider-native session ID without representing the same source.

Provider storage may also move because of configuration, mount points, environment changes, or operating-system differences.

Using an absolute file system path as logical identity would make one logical store appear to change identity whenever its locator changes.

Hard-coding provider-internal session directories into user configuration would also expose implementation details that should remain inside provider adapters.

At the same time, an ordinary single-store environment should work without explicit configuration.

## Decision

`agent-sessions` will model each independently addressable provider storage or access boundary as a named source instance.

A source instance has at least:

- a stable logical ID;
- a provider;
- an access boundary;
- a provider-level locator when required.

Examples may include:

```text
codex-wsl
codex-windows-app
claude-wsl
```

The identity rules are:

| Aspect | Rule |
| --- | --- |
| Source instance ID | A logical alias that is not derived from a local absolute path. |
| Source scope | A logical source is identified within a source instance. |
| Provider-native session ID | Not assumed to be globally unique across instances. |
| File system path or mount point | A locator only. |

Conceptually:

```text
provider
source instance
provider-native source ID
```

## Provider-level configuration

When a provider exposes a configurable home or equivalent root, user configuration should normally identify that provider-level boundary rather than a provider-internal session subdirectory.

Prefer provider-level concepts over internal paths such as a particular `sessions/` directory.

Examples include:

```text
Codex home
Claude Code configuration root
export archive
```

The provider adapter owns knowledge of the provider's internal layout.

## Resolution precedence

Source configuration is resolved in this order:

```text
1. explicit CLI option
2. agent-sessions user configuration
3. provider environment
4. provider default
```

This allows normal single-store environments to work without configuration while supporting explicit multi-store environments.\
Machine-specific paths belong in machine-local user configuration, not portable repository files.\
The exact configuration syntax and operating-system-specific location remain implementation decisions until the first configuration contract is defined.

## Access methods

Logical source identity is separate from the mechanism used to access a source.\
For example, one Codex instance may be accessed through a documented provider interface while another may require read-only file system access.\
Changing the access method does not necessarily create a new logical instance if it still refers to the same underlying provider store.

An access method is acceptable only when it preserves the `agent-sessions` read-only contract.\
A documented provider interface is preferred where suitable, but official status does not override the no-write requirement.

## Automatic default instance

When no explicit instance is configured, an adapter may resolve one default source from documented provider environment variables and defaults.

This supports an ordinary command without requiring user configuration:

```text
agent-sessions list --provider codex
```

When multiple independently addressable stores are needed, they should receive explicit stable instance IDs.

## Consequences

### Positive

- Multiple stores for one provider can coexist without identity collisions.
- Windows-native and WSL stores can remain distinct on one physical computer.
- Provider-native IDs remain useful without pretending to be globally unique.
- Machine path changes do not automatically change consumer-facing identity.
- Provider-internal layouts remain encapsulated behind adapters.
- Normal single-store environments remain configuration-free.
- Access strategy can evolve separately from source identity.

### Negative

- Users with multiple stores must maintain stable instance IDs.
- Renaming an instance is logically significant to downstream consumers unless an explicit migration mechanism exists.
- Nonstandard and multi-store environments require machine-local configuration.
- Automatic discovery cannot reliably infer whether two separately located stores should represent one logical instance.

These costs are preferable to silently conflating unrelated stores.

## Alternatives considered

### Treat one provider as one source instance

Rejected.\
The same provider may have multiple independent stores on one computer.

### Use absolute paths as identities

Rejected.\
Paths are environment-specific locators and may change because of mounts, home-directory configuration, operating systems, or migrations.

### Configure internal session directories directly

Rejected as the normal contract.\
It leaks provider implementation details into user configuration and makes provider layout changes harder to isolate within adapters.\
Low-level diagnostic overrides may exist later without defining the normal model.

### Generate opaque identifiers from paths

Rejected.\
Hashing a locator hides the path but retains the same identity instability: a path change would still create a new logical identity.

## Revisit conditions

Revisit this decision if:

- a provider introduces a stable store identifier that can replace user-managed logical aliases;
- one provider home contains multiple independently addressable logical stores that this model cannot represent cleanly;
- cross-machine synchronization becomes an explicit product requirement;
- source instance renames require a supported identity migration mechanism.
