# Contributing Guide

Follow this workflow when you maintain this repository or contribute a change.

## Track the work in an Issue

Use an existing [Issue](https://github.com/mtk177a/agent-sessions/issues) or open one before work starts.\
Use the [Issue template](.github/ISSUE_TEMPLATE/default.md) to describe the purpose, scope, and conditions for completion.\
Skip the Issue only when the maintainer explicitly says to.

## Check before changing files

Read the relevant [repository instructions](AGENTS.md) and the files the change affects.\
Check the working tree so that you can keep existing changes separate.\
When you change a maintained English document, check its Japanese reference under the [language policy](docs/localization.md#maintaining-japanese-references).

## Verify the change

Check the changed behavior, its failure cases, and any relevant data safety requirements.\
For implementation changes, run the applicable [development checks](README.md#development-validation).

Check the diff from the repository root before committing.

```sh
git diff --check
```

In the PR, report the checks you ran, their results, and any important checks you did not run.

## Commit and open a pull request

Give each commit one purpose that a reviewer can understand and verify.\
Use the [PR template](.github/pull_request_template.md) to describe the result and the evidence.\
Use `Closes #123` when the PR completes the Issue and `Refs #123` for a reference only.\
Follow the [language policy](docs/localization.md#languages-to-use) for Issues and PRs.

### Commit messages and pull request titles

Use Conventional Commits for commit messages you write or edit and for PR titles.\
A PR title describes the overall change.\
A commit subject describes the change in that commit.\
Unedited GitHub-generated merge messages and Dependabot titles and messages are exempt.\
Apply the format when you edit a generated title or message.

#### Form

```text
<type>: <English summary>
<type>(<scope>): <English summary>
<type>!: <English summary>
<type>(<scope>)!: <English summary>
```

- State the result in short, specific English without a final period.
- Use `!` for breaking changes and explain their impact in the commit body or PR description.
- Choose the type by the main purpose of the change.\
  Supporting tests and documentation use the type of the change they support.
- Consider splitting independent changes.\
  When changes belong together, name the primary purpose in the title and explain the relationship in the PR.

#### Types

| Type | Primary purpose |
| --- | --- |
| `feat` | Add a capability or behavior for users or maintainers. |
| `fix` | Correct a confirmed defect or restore intended behavior. |
| `docs` | Improve explanations or reference material while preserving product behavior. |
| `chore` | Update dependencies, configuration values, or maintenance policy. |
| `refactor` | Reorganize the implementation or instructions while preserving behavior. |
| `test` | Improve test coverage or checks while preserving product behavior. |
| `ci` | Change how or when CI runs. |

Choose the type for a CLI or provider change by its effect on the published behavior.\
Use `chore` for intentional maintenance updates that add no capability and correct no defect.

#### Scopes

Use a short scope when the change has a clear target, such as a capability, provider, or policy.\
Use the same name for changes to the same target, including related tests and documentation.\
Omit the scope when there is no single target.

#### Examples

```text
feat: find sessions by local Git repository
fix(codex): normalize command working directories
docs: clarify repository verification steps
```
