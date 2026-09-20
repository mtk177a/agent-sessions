# maintain-japanese-references evaluation

## Provenance and purpose

This repository-local Skill adapts the responsibility and boundary of [`mtk177a/skills`' `maintain-japanese-references`](https://github.com/mtk177a/skills/tree/b32b606de2650d140d000150bb67a66f70de521e/.agents/skills/maintain-japanese-references) (MIT, revision `b32b606de2650d140d000150bb67a66f70de521e`).\
The adapted artifact uses this repository's fixed nine pairs and its `docs/localization.md` policy.\
Only the upstream `SKILL.md` and its `evals/README.md` informed the adaptation; no upstream scripts, assets, or executable material were copied.

The cases below test translation decisions and routing.\
Use independently constructed fictional text, never real session data or private material.\
For each case, give a blank-context executor only the candidate `SKILL.md`, the request, and the listed fixture content.\
Keep the expected results below out of the executor input.\
A separate reviewer compares output and any edits with the canonical text and expected result; an executor's self-report alone is not a pass.

## Cases

| Case | Executor input | Expected result and grading |
| --- | --- | --- |
| A: meaning changes | An in-scope English paragraph changes `should` to `must` and narrows an exception; provide its existing Japanese reference. Ask to maintain the pair. | The Japanese reference preserves both changes, adds no rule, and the report names the reason. Compare both passages and the diff. |
| B: meaning stays the same | An in-scope English file changes only source-line wrapping; provide an aligned Japanese reference. | The Japanese file remains byte-for-byte unchanged and the report explains why. Inspect the diff and report. |
| C: invented meaning | An in-scope English paragraph describes an optional verification step; provide a Japanese draft that makes it mandatory and adds a new example. Ask to align the pair. | The mandatory wording and extra example are removed or corrected, with no English-source edit. Compare claims sentence by sentence. |
| D: ambiguity | An in-scope English term has two materially different possible meanings and no disambiguating context; provide its Japanese reference. | The executor identifies the affected pair and uncertainty rather than choosing one meaning, while completing independent work if supplied. Inspect the report and edits. |
| E: unrelated pair | Provide a meaningful change to one maintained English file, its Japanese counterpart, and an unrelated maintained pair. | Only the affected Japanese counterpart changes. Inspect the complete diff and report. |
| F: tracker request | Ask for an Issue or pull request body with no changed maintained English file. | The Skill does not author, translate, or edit tracker content or repository files under its translation workflow. Inspect output and diff. |

## Procedure and acceptance

Run a static check of frontmatter, the nine-pair boundary, canonical-source notice requirement, ambiguity handling, and scope exclusions.\
Then run cases A–F once in isolated executor contexts, with a separate reviewer applying the stated grading.\
An incorrect translation, invented requirement, unrequested file edit, or responsibility-boundary violation fails acceptance.\
Correct the Skill or a defective case and rerun only affected cases; repeat or compare models only when results are ambiguous or unstable.\
Record actual prompts, outputs, diffs, reviewer findings, and unexecuted cases in the implementation report rather than treating this document as proof that a case passed.
