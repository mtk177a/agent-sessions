# Evaluate maintain-japanese-references

Use this procedure after you change the Skill to check translation decisions and responses to requests outside its scope.

## Evaluation procedure

### 1. Check the Skill text

Examine [SKILL.md](../SKILL.md) for these items:

- Valid frontmatter.
- A scope limited to the [maintained pairs](../../../../docs/localization.md#maintained-files).
- An instruction to include the required English-source notice in Japanese references.
- Instructions for ambiguous meaning.
- Exclusion of ADRs, tracker writing, general translation, and English-source edits.

### 2. Prepare the inputs

Use independently constructed fictional text, never real session data or private material.

For each case, give the executor only:

- The candidate `SKILL.md`.

- The request and fixtures described under **Input**.

- These sections of `docs/localization.md`, with their source path:
  - [Maintained files](../../../../docs/localization.md#maintained-files).
  - [Required reference notice](../../../../docs/localization.md#required-reference-notice).
  - [Update handling](../../../../docs/localization.md#update-handling).

Keep the expected results out of the executor's input.

### 3. Run the cases

Run cases A–F once each.\
Start each executor with no prior conversation context.

### 4. Judge the results

A separate reviewer must compare the output, complete diff, and English source with each expected result.\
The executor's report alone does not establish a pass.

Fail the evaluation for an incorrect translation, invented requirement, unrequested file edit, or action outside the Skill's scope.

Correct the Skill or a defective case, then rerun only affected cases.\
Repeat runs or compare models only when results are ambiguous or unstable.

### 5. Record the results

In the implementation report, record:

- Actual prompts and outputs.
- Diffs and reviewer findings.
- Cases not run.

This procedure is not evidence that a case passed.

## Evaluation cases

### A. English meaning changes

- **Input**

  An in-scope English paragraph changes `should` to `must` and narrows an exception.\
  Supply its existing Japanese reference and ask the executor to maintain the pair.

- **Expected result**

  The Japanese reference preserves both changes and adds no rule.\
  The report explains the reason for the update.

### B. English meaning stays the same

- **Input**

  An in-scope English file changes only source-line wrapping.\
  Supply a Japanese reference with the same meaning.

- **Expected result**

  The Japanese file stays byte-for-byte unchanged.\
  The report explains why.

### C. The Japanese draft adds meaning

- **Input**

  An in-scope English paragraph describes an optional verification step.\
  The Japanese draft makes it mandatory and adds an example.\
  Ask the executor to align the pair.

- **Expected result**

  The mandatory wording and extra example are removed or corrected.\
  The English source stays unchanged.

### D. English meaning is ambiguous

- **Input**

  An in-scope English term has two materially different possible meanings and no context that distinguishes them.\
  Supply its Japanese reference.

- **Expected result**

  The executor reports the affected pair and uncertainty instead of choosing one meaning.\
  It completes independent work if supplied.

### E. An unrelated pair is included

- **Input**: Supply a meaningful change to one maintained English file, its Japanese reference, and an unrelated maintained pair.
- **Expected result**: The executor changes only the affected Japanese reference.

### F. The request asks for tracker content

- **Input**: Ask for an Issue or pull request body with no changed maintained English file.
- **Expected result**: In this workflow, the executor does not write, translate, or edit tracker content or change repository files.

## Source

This Skill adapts [maintain-japanese-references from mtk177a/skills](https://github.com/mtk177a/skills/tree/b32b606de2650d140d000150bb67a66f70de521e/.agents/skills/maintain-japanese-references) (MIT, revision `b32b606de2650d140d000150bb67a66f70de521e`).\
Only its `SKILL.md` and `evals/README.md` informed the adaptation.
