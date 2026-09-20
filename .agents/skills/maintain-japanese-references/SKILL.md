---
name: maintain-japanese-references
description: Use in agent-sessions when a maintained English README, agent guidance, repository document, or repository-local Skill is added or changed and its Japanese reference must be created, synchronized, or explicitly left unchanged. Preserve English canonical meaning; not for ADRs, Issue or pull request authoring, general translation, or changing the English source.
license: MIT
---

# Maintain Japanese References

## Purpose and scope

Keep the Japanese references listed in `docs/localization.md` aligned with their English canonical files.\
Japanese references help comprehension and review but do not define requirements or permissions.\
If the versions differ, the English file prevails.

This Skill covers only the nine explicit pairs in `docs/localization.md`.\
It does not author Issues or pull requests, translate ADRs or other unlisted files, or change an English canonical file as part of translation maintenance.

## Workflow

1. Identify added or changed English files among the maintained pairs from the request and current diff. Read each changed section and its context, plus the existing Japanese counterpart if present.
2. Decide whether each English change affects meaning needed to use the document correctly. Requirements, scope, exceptions, permissions, prohibitions, safety conditions, procedures, validation, links, and relevant structure may be meaning-bearing.
3. If meaning changed, create or update that pair's Japanese reference in the same change. Preserve the force of `must`, `should`, and `may`, conditions, exceptions, uncertainty, and the relationship between sections.
4. If meaning did not change, leave the Japanese file untouched and report the reason for leaving it unchanged. Record that reason in the pull request's Validation or Risks / Follow-up section when a pull request is prepared.
5. Check each affected pair for semantic alignment, established terminology, the canonical-source notice, a correct relative link, protected code and identifiers, Markdown structure, and unintended edits.

Each Japanese reference must identify its English source and state that the English file is canonical and prevails if the two differ.\
Use only the pairs affected by the English change; do not rewrite unrelated translations.\
Do not add an explanation, example, policy, or claim absent from the English source.\
If English meaning or established Japanese terminology is materially ambiguous, report the pair and ambiguity instead of guessing; continue with independent pairs when possible.

## Report

Name both file paths for every affected pair and report whether the Japanese reference was created, updated, or left unchanged and why.\
Include checks performed, unresolved ambiguities, and pairs not verified.

## Authority and dependencies

This Skill grants no authority to commit, push, publish, change permissions, or send content externally.\
It requires no external translation service, dependency, script, or companion Skill.\
When a Japanese-writing Skill is also used, this Skill controls meaning and scope; the writing Skill may improve expression only within those limits.
