# 言語方針

> **注記:** [英語版](../localization.md) が正式版です。
> この日本語版は参考訳です。
> 内容に差異がある場合は英語版を優先します。

このリポジトリの公開文書とリポジトリ規則は、英語版を正式版とします。\
保守対象の日本語版は、内容の理解とレビューに使う参考訳であり、独立した要件や権限を定めません。\
日本語版と英語版の内容が異なる場合は、英語版を優先します。

## 保守対象の参考訳

| 正式な英語版 | 日本語参考訳 |
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

各参考訳には英語版へのリンクを設け、英語版が正式版であり、内容に差異がある場合は英語版を優先することを明記します。\
`AGENTS-ja.md` は `AGENTS.md` の参考訳であり、独立したエージェント向け指示や権限を定めません。\
`docs/decisions/` 配下の ADR は保守対象の参考訳に含めません。

## 参考訳の更新

保守対象の英語版を追加または変更した場合は、リポジトリ専用の `maintain-japanese-references` Skill を使って対応する日本語版を確認します。\
文書の正しい利用に必要な意味が変わる場合は、同じ変更内で日本語参考訳を作成または更新します。\
意味には、要件、対象範囲、例外、権限、禁止事項、手順、検証、リンク、関連する文書構造を含みます。

英語版の変更が日本語版に反映すべき意味を変えない場合は、参考訳を変更せず、その理由を Pull Request の「検証」または「リスク / フォローアップ」に記録します。\
変更対象となった組だけを扱い、無関係な参考訳を書き直しません。\
英語版の意味や既存の訳語を確定できない場合は、推測せず、対象となる組と不明点を報告します。

## リポジトリで使う言語

コード、識別子、コードコメント、CLI のヘルプと診断メッセージ、リリースノート、コミットメッセージの要約は、引き続き英語を既定とします。\
参考訳によって、公開済みの製品インターフェースや実行時の動作は変わりません。

保守者が作成する Issue と Pull Request は、タイトルと `Summary` を英語で書きます。\
その他の本文とコメントは日本語を既定としますが、必要に応じて英語でも書けます。\
本文全体を英語と日本語の両方で記載する必要はありません。\
外部のコントリビューターに、保守者向けの日本語既定の運用は要求しません。
