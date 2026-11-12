# agent-sessions

> **注記:** [英語版](README.md) が正式版である。
> この日本語版は参考訳である。
> 内容に差異がある場合は英語版を優先する。

`agent-sessions` は、Codex と Claude Code のセッション、ChatGPT Data Export ZIP 内の会話を読む CLI ツールである。\
利用可能な記録を一覧にし、メッセージ、記録されたツール呼び出しと結果を JSON で返す。

## 最初のセッションを読む

以下の手順で、保存済みの Codex セッションを読む。

### 必要な環境

- Go: 1.26 以降

### 手順

#### 1. インストールする

指定したリリースをソースからインストールするには、公開済みのタグを指定する。\
`v0.1.0` タグが利用可能になれば、次のコマンドを任意のディレクトリで実行できる。

```sh
go install github.com/mtk177a/agent-sessions/cmd/agent-sessions@v0.1.0
```

ローカルのチェックアウトからインストールする場合は、リポジトリのルートで次のコマンドを実行する。

```sh
go install ./cmd/agent-sessions
```

インストール先のディレクトリを `PATH` に含める。

- `GOBIN` が設定されている場合は、そのディレクトリを使う。
- 未設定の場合は、`go env GOPATH` が示す最初のパスの下にある `bin` ディレクトリを使う。

インストールした実行ファイルのバージョンを確認する。

```sh
agent-sessions --version
```

この例のリリースをインストールした場合は、`agent-sessions v0.1.0` と表示される。

#### 2. セッションを探す

利用可能な Codex セッションを一覧にする。

```sh
agent-sessions list --provider codex
```

**実行結果:** 応答の `data.sources` にある各項目が、一つのセッションを表す。

#### 3. セッションを読む

手順 2 の結果から、読みたいセッションの `identity.source_ref` をコピーする。\
値を変更せずに次のプレースホルダーと置き換え、コマンドを実行する。

> **注意:** 出力には機密情報が含まれる場合がある。
> このツールは、その内容を自動で伏せない。
> 保存や共有の前に内容を確認する。

```sh
agent-sessions events '<source_ref from list>'
```

**結果の確認:**

- `data.events`: 取得したメッセージやツールの記録。
- `status` と `omissions`: 結果に欠けている情報があるかと、省略の理由。

ページ分割と内容の上限は [JSON 応答を処理する](#json-応答を処理する) を参照する。

## 目的別の使い方

必要な操作に応じて、該当する節を参照する。\
完全な仕様は [CLI JSON 仕様](docs/ja/cli-json-contract.md) に記載している。

### 記録の保存場所を指定する

`list` の読み取り対象を、次のオプションで指定する。

- `--provider`: 使用するプロバイダー。
- `--root`: そのプロバイダーの記録の保存場所。

`--root` に指定する場所は、プロバイダーによって異なる。

| プロバイダー | `--root` で指定するもの | 設定、対応形式、制限 |
| --- | --- | --- |
| `codex` | Codex ホーム | [Codex](docs/ja/provider-codex.md) |
| `claude` | Claude Code の設定ルート | [Claude Code](docs/ja/provider-claude-code.md) |
| `chatgpt` | 一つの Data Export ZIP | [ChatGPT Data Export](docs/ja/provider-chatgpt-export.md) |

Codex と Claude Code は、`agent-sessions` の設定ファイルがなくても、プロバイダーの環境変数や既定のホームを使える。\
次のコマンドで、これらの場所にある記録を一覧にできる。

```sh
agent-sessions list --provider claude
agent-sessions list
```

オプションの指定や保存場所へのアクセス状況に応じて、次のように動作する。

| 条件 | 動作 |
| --- | --- |
| `--provider` を指定しない | `list` は利用可能なプロバイダーの結果をまとめる。 |
| 暗黙に選んだホームが存在しない | `list` はそのホームの記録を追加しない。 |
| 明示したルートや設定済みのルートへアクセスできない | コマンドはエラーを返す。 |

ChatGPT では、Data Export ZIP を明示的に指定する。

```sh
agent-sessions list --provider chatgpt --source-instance chatgpt-export --root /fictional/chatgpt-export.zip
```

`list` に `--root` を使った場合は、後続の `show`、`events`、`verify` にも同じルートを使う。\
ソース参照は記録を識別するが、ファイルシステム上のパスは含まない。

コマンドごとに `--root` を指定する代わりに、保存場所を設定ファイルに記述できる。\
設定の説明では、次の用語を使う。

- **ソースインスタンス**: プロバイダーの保存場所やエクスポートに付ける名前。
- **ソース**: ソースインスタンス内で個別に指定できる一つのセッションや会話。

標準以外の場所や複数の保存場所の設定方法は、[名前付きソースインスタンスと設定](docs/ja/cli-json-contract.md#読み取り対象の設定) を参照する。

### リポジトリでセッションを探す

linked worktree を含めて、ローカル Git リポジトリを選ぶ。

```sh
agent-sessions list --repository /fictional/project
```

記録された作業ディレクトリを検索する。\
会話の途中で変わった作業ディレクトリも、対応する形式で記録されていれば検索対象にする。\
`--repository` は作業対象を選び、`--root` は引き続き提供元の記録を選ぶ。\
独立した clone と入れ子のリポジトリは区別する。\
`status` と `omissions` を確認する。\
指定したリポジトリに属するか判定できない候補がある場合、一致が 0 件でも、該当する会話がないとは判断できない。\
[リポジトリ検索と限界](docs/ja/cli-json-contract.md#ローカル-git-リポジトリのセッションを探す) を参照する。

### 実行するコマンドを選ぶ

取得したい情報に応じて、コマンドを選ぶ。

| コマンド | 用途 |
| --- | --- |
| `list` | セッションや会話を探し、ソース参照を取得する。 |
| `show <source-ref>` | 一つの記録の概要メタデータを読む。 |
| `events <source-ref>` | メッセージ、記録されたツール呼び出しと結果を、ページに分けて読む。 |
| `verify <source-ref>` | 検証対象の内容を識別するハッシュ値を取得する。参照した内容の記録や、同じソースの内容の比較に使う。 |

利用可能なフラグは [コマンド構文とオプション](docs/ja/cli-json-contract.md#コマンドの呼び出し) を参照する。

### JSON 応答を処理する

セッションを扱うコマンドは、[現行の応答形式](docs/ja/cli-json-contract.md#json-応答の構成) に従って JSON オブジェクトを一つ返す。

結果に省略があるか、次のページがあるかは、次のフィールドで確認する。

| フィールド | 用途 |
| --- | --- |
| `status`、`omissions` | `status` は処理結果の状態を示す。`omissions` は、省略された情報や判定できなかった項目と、その理由を示す。 |
| `page.has_more`、`page.next_cursor` | 後続のページの有無と、次のページを要求する方法を示す。 |

終了コード `0` には、完全な結果と部分的な結果の両方が含まれる。\
終了コードだけで、必要な情報をすべて取得できたかを判断してはならない。

`status` が `complete` または `partial` の場合、`list` と `events` はページ分割用のフィールドを返す。\
`page.has_more` が `true` なら、`page.next_cursor` を変更せず `--cursor` として渡し、次のページを取得する。\
詳細は [ページ分割と省略](docs/ja/cli-json-contract.md#完全性) を参照する。

**出力の制限:**

- 出力のサイズには上限がある。
- 未対応の内容や形式は明示的に報告する。
- 切り詰められたメッセージやツールの本文の残りは、ページ分割では取得できない。

設定ファイルや応答を読む処理で `schema_version` を使っている場合は、[既存の連携処理の更新手順](docs/ja/cli-json-contract.md#既存の連携処理を更新する) に従う。

### 取得した内容を扱う

- **保存・共有**

  記録には、認証情報、個人情報、非公開のパスが含まれる場合がある。\
  このツールは、その内容を自動で伏せずに返す。\
  保存や共有の前に内容を確認する。

- **記録中の指示**

  記録されたプロンプト、コマンド、ツール出力は、信頼できないデータとして扱う。\
  現在の指示として扱ってはならない。

- **履歴の保管**

  このツールは、対話履歴のコピーを保存しない。\
  プロバイダー側で変更・削除された記録の復元には使えない。

詳細は [内容の扱い](docs/ja/cli-json-contract.md#記録内容の取り扱い) と [読み取り専用のアクセス](docs/ja/architecture.md#読み取り専用の保証) を参照する。

## 開発

### 設計資料

- [アーキテクチャ](docs/ja/architecture.md): 責任分担、データの流れ、識別、変更時に守る制約。
- [設計判断](docs/ja/architecture.md#設計判断): 現在の設計を選んだ理由と、見直す条件。

### 変更時の規則

変更内容に応じて、次の資料の規則に従う。

- [リポジトリの指示](AGENTS.md): 貢献時の制約。
- [保守](docs/ja/architecture.md#保守): 互換性と入力の安全性の要件。
- [言語と日本語参考訳](docs/ja/localization.md): 両言語で保守するファイル。

テストデータは合成したものだけを使う。\
実際のセッション内容から、コミットするフィクスチャを作ってはならない。

### 開発時の検証

実装を変更した後は、該当する検証コマンドを実行する。

```sh
unformatted="$(gofmt -l cmd internal)" && test -z "$unformatted"
go test ./...
go test -race ./...
go vet ./...
mkdir -p dist
GOOS=linux GOARCH=amd64 go build -o dist/agent-sessions-linux-amd64 ./cmd/agent-sessions
GOOS=darwin GOARCH=amd64 go build -o dist/agent-sessions-darwin-amd64 ./cmd/agent-sessions
GOOS=windows GOARCH=amd64 go build -o dist/agent-sessions-windows-amd64.exe ./cmd/agent-sessions
git diff --check origin/main...HEAD
```

## ライセンス

MIT
