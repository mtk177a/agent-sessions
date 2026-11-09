# Language policy

Use this policy to choose the language for repository work and maintain Japanese references.

## Languages to use

Use the following languages for each type of content:

| Content | Language rule |
| --- | --- |
| Public documentation and repository rules | English is canonical. |
| Code, identifiers, code comments, CLI help and diagnostics, release notes, and commit-message summaries | English by default. |
| Maintainer-created Issues and pull requests: titles and `Summary` sections | English. |
| Maintainer-created Issues and pull requests: remaining body and comments | Japanese by default. English may be used when appropriate. |

For maintainer-created Issues and pull requests, full duplication of the body in both languages is not required.

External contributors are not required to follow the maintainer's Japanese-language default.

## Maintaining Japanese references

### Role of references

The maintained Japanese versions are reference translations for comprehension and review.\
They do not define independent requirements or authority.\
If a Japanese reference differs from its English source, the English version prevails.

`AGENTS-ja.md` is a reference to `AGENTS.md`.\
It does not provide separate agent instructions or permissions.

Reference translations do not change the product's published interfaces or runtime behavior.

### Maintained files

The following English/Japanese pairs are maintained:

| Canonical English file | Japanese reference |
| --- | --- |
| `README.md` | `README.ja.md` |
| `AGENTS.md` | `AGENTS-ja.md` |
| `docs/architecture.md` | `docs/ja/architecture.md` |
| `docs/provider-chatgpt-export.md` | `docs/ja/provider-chatgpt-export.md` |
| `docs/provider-claude-code.md` | `docs/ja/provider-claude-code.md` |
| `docs/cli-json-contract.md` | `docs/ja/cli-json-contract.md` |
| `docs/provider-codex.md` | `docs/ja/provider-codex.md` |
| `docs/localization.md` | `docs/ja/localization.md` |
| `.agents/skills/maintain-japanese-references/SKILL.md` | `.agents/skills/maintain-japanese-references/SKILL-ja.md` |

ADRs under `docs/decisions/` are outside the maintained reference set.

### Required reference notice

Each reference must include the following:

- A link to the corresponding English source.
- A statement that the English version is canonical.
- A statement that the English version prevails if the versions differ.

### Update handling

When a maintained English file is added or changed, use the repository-local `maintain-japanese-references` Skill to review its Japanese counterpart.

Meaning includes requirements, scope, exceptions, permissions, prohibitions, procedures, validation, links, and relevant document structure.

Use the review result to choose the action:

| Review result | Required action |
| --- | --- |
| The English change affects meaning needed to use the document correctly. | Create or update the Japanese reference in the same change. |
| The English change does not affect Japanese meaning. | Leave the reference unchanged. Record the reason in the pull request's Validation or Risks / Follow-up section. |
| The English meaning or an established translation is materially ambiguous. | Do not guess. Report the affected pair and uncertainty. |

Change only the affected pairs.\
Do not rewrite unrelated references.
