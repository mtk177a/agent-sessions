# 言語方針

> **注記:** [英語版](../localization.md) が正式版である。
> この日本語版は参考訳である。
> 内容に差異がある場合は英語版を優先する。

この文書は、リポジトリで使う言語の選択と、日本語参考訳の保守に使う。

## 使用する言語

対象ごとに、次の言語を使う。

| 対象 | 使用言語の規則 |
| --- | --- |
| 公開文書とリポジトリ規則 | 英語版を正式版とする。 |
| コード、識別子、コードコメント、CLI のヘルプと診断メッセージ、リリースノート、コミットメッセージの要約 | 英語を既定とする。 |
| 保守者が作成する Issue と Pull Request のタイトル、`Summary` | 英語で書く。 |
| 保守者が作成する Issue と Pull Request のその他の本文、コメント | 日本語を既定とする。必要に応じて英語でも書ける。 |

保守者が作成する公開 Issue と Pull Request では、本文全体を英語と日本語の両方で書かない。

外部のコントリビューターには、日本語を既定とする運用を求めない。

## 日本語参考訳の保守

### 参考訳の位置付け

保守対象の日本語版は、内容の理解とレビューに使う参考訳である。\
独立した要件や権限を定めるものではない。\
日本語版と英語版の内容が異なる場合は、英語版を優先する。

`AGENTS-ja.md` は `AGENTS.md` の参考訳である。\
独立したエージェント向け指示や権限を定めるものではない。

参考訳によって、公開済みの製品インターフェースや実行時の動作は変わらない。

### 保守対象

次の日英の組を保守する。

| 正式な英語版 | 日本語参考訳 |
| --- | --- |
| `README.md` | `README.ja.md` |
| `AGENTS.md` | `AGENTS-ja.md` |
| `CONTRIBUTING.md` | `CONTRIBUTING-ja.md` |
| `docs/architecture.md` | `docs/ja/architecture.md` |
| `docs/provider-chatgpt-export.md` | `docs/ja/provider-chatgpt-export.md` |
| `docs/provider-claude-code.md` | `docs/ja/provider-claude-code.md` |
| `docs/cli-json-contract.md` | `docs/ja/cli-json-contract.md` |
| `docs/provider-codex.md` | `docs/ja/provider-codex.md` |
| `docs/localization.md` | `docs/ja/localization.md` |
| `.agents/skills/maintain-japanese-references/SKILL.md` | `.agents/skills/maintain-japanese-references/SKILL-ja.md` |

`docs/decisions/` 配下の ADR は保守対象の参考訳に含めない。

### 参考訳への記載事項

各参考訳には、次の内容を記載する。

- 対応する英語版へのリンク。
- 英語版が正式版であること。
- 内容に差異がある場合は英語版を優先すること。

### 更新時の対応

保守対象の英語版を追加または変更した場合は、リポジトリ専用の `maintain-japanese-references` Skill を使って対応する日本語版を確認する。

文書の正しい利用に必要な意味には、要件、対象範囲、例外、権限、禁止事項、手順、検証、リンク、関連する文書構造を含む。

確認の結果に応じて、次のように対応する。

| 確認結果 | 必要な対応 |
| --- | --- |
| 文書の正しい利用に必要な意味が変わる。 | 同じ変更内で日本語参考訳を作成または更新する。 |
| 日本語版に反映すべき意味は変わらない。 | 参考訳を変更せず、その理由を Pull Request の「検証」に記録する。 |
| 英語版の意味や既存の訳語を確定できない。 | 推測せず、対象となる組と不明点を報告する。 |

変更対象となった組だけを扱う。\
無関係な参考訳を書き直さない。
