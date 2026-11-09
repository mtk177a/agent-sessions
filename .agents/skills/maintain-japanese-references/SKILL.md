---
name: maintain-japanese-references
description: Maintain Japanese references in agent-sessions when a maintained English file is added or changed. Do not use this Skill for ADRs, tracker writing, general translation, or English-source edits.
license: MIT
---

# Maintain Japanese References

## Scope

Use only the [maintained pairs](../../../docs/localization.md#maintained-files).\
Keep the English source unchanged during translation maintenance.

Do not write Issues or pull requests in this workflow.\
For those requests alone, report that this Skill does not apply.

## Procedure

### 1. Select affected pairs

From the request and diff, select only added or changed English files in the maintained list.\
Read each changed section and its context.\
If a Japanese reference exists, read it.

### 2. Decide whether to update

Use the [policy definition of meaning](../../../docs/localization.md#update-handling) to select the action:

| Assessment | Action |
| --- | --- |
| Meaning changes | Create or update the Japanese reference in the same change. |
| Meaning does not change | Leave the Japanese file unchanged. Report why. When you prepare a pull request, record why under Validation or Risks / Follow-up. |
| English meaning or established Japanese terminology is materially ambiguous | Do not guess. Report the pair, ambiguity, and missing context. Where possible, continue work on independent pairs. |

If materially different interpretations remain, report the missing context even when neutral wording or transliteration is possible.

### 3. Write the necessary references

Change only the Japanese references that need an update.

- Keep the force of `must`, `should`, and `may`.
- Keep conditions, exceptions, uncertainty, and the relationships between sections.
- Do not add explanations, examples, rules, or claims that are not in the English source.

### 4. Check each pair

- Make sure that the Japanese meaning and established terms agree with the English source.
- Make sure that the reference has the [required English-source notice](../../../docs/localization.md#required-reference-notice).
- Examine code, identifiers, relative links, Markdown, and the diff for unintended changes.

### 5. Report the result

For each affected pair, report:

- Both file paths.
- Whether you created, updated, or left the Japanese file unchanged, and why.
- Checks completed, unresolved ambiguity, and pairs not verified.
