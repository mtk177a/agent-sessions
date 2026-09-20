# Localization

English is the canonical language for public documentation and repository rules in this repository.\
The maintained Japanese versions are reference translations for comprehension and review, not independent requirements or sources of authority.\
If a Japanese reference differs from its English source, the English version prevails.

## Maintained references

| Canonical English file | Japanese reference |
| --- | --- |
| `README.md` | `README.ja.md` |
| `AGENTS.md` | `AGENTS-ja.md` |
| `docs/architecture.md` | `docs/ja/architecture.md` |
| `docs/chatgpt.md` | `docs/ja/chatgpt.md` |
| `docs/claude.md` | `docs/ja/claude.md` |
| `docs/cli-json-contract.md` | `docs/ja/cli-json-contract.md` |
| `docs/codex.md` | `docs/ja/codex.md` |
| `docs/localization.md` | `docs/ja/localization.md` |
| `.agents/skills/maintain-japanese-references/SKILL.md` | `.agents/skills/maintain-japanese-references/SKILL-ja.md` |

Each reference must identify and link to its English source and state that the English version prevails if the two differ.\
`AGENTS-ja.md` is a reference to `AGENTS.md`; it does not provide separate agent instructions or permissions.\
ADRs under `docs/decisions/` are outside the maintained reference set.

## Updating references

When a maintained English file is added or changed, use the repository-local `maintain-japanese-references` Skill to review its Japanese counterpart.\
If the change affects meaning needed to use the document correctly, create or update the Japanese reference in the same change.\
Meaning includes requirements, scope, exceptions, permissions, prohibitions, procedures, validation, links, and relevant document structure.

If the English change does not affect Japanese meaning, leave the reference unchanged and record the reason in the pull request's Validation or Risks / Follow-up section.\
Change only the affected pairs; do not rewrite unrelated references.\
If the English meaning or an established translation is materially ambiguous, report the affected pair and uncertainty instead of guessing.

## Repository language

Code, identifiers, code comments, CLI help and diagnostics, release notes, and commit-message summaries remain English by default.\
The product's published interfaces and runtime behavior are unaffected by reference translations.

Maintainer-created Issue and pull request titles and their `Summary` sections are in English.\
The remaining body and comments are in Japanese by default, but may be written in English when appropriate.\
Full duplication of the body in both languages is not required.\
External contributors are not required to follow the maintainer's Japanese-language default.
