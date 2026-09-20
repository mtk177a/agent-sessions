# agent-sessions

> **注記:** [英語版](README.md) が正式版です。
> この日本語版は参考訳です。
> 内容に差異がある場合は英語版を優先します。

`agent-sessions` は、複数のプロバイダーが保持するコーディングエージェントのセッション記録を、ローカルで読み取り専用で参照できるようにします。

プロバイダーが保持する対話記録を検出し、正規化した機械可読のインターフェースを通じて公開します。\
記録の解釈と永続的な処理状態は、下流の利用側が扱います。

> 状況: プロバイダー共通の CLI 基盤と、読み取り専用の Codex、Claude Code、ChatGPT Data Export アダプターが、安定した `v1` JSON 仕様を実装しています。

## ビルド

Go 1.26 以降が必要です。

```sh
go build ./cmd/agent-sessions
```

外部の Go モジュールに依存しない、単独の実行ファイルが生成されます。

## ローカルインストール

リポジトリのチェックアウトから、現在使用している Go のバイナリディレクトリへ実行ファイルをインストールします。

```sh
go install ./cmd/agent-sessions
```

`GOBIN` が設定されていればその場所を使い、未設定なら Go ツールチェーンの既定のバイナリディレクトリを使います。

## コマンド

機械可読インターフェースは 4 つの操作で構成されます。

```text
agent-sessions list [--config PATH] [--provider ID [--source-instance ID] [--root PATH]] [--limit N] [--cursor TOKEN]
agent-sessions show [--config PATH] [--root PATH] <source-ref>
agent-sessions events [--config PATH] [--root PATH] [--limit N] [--cursor TOKEN] <source-ref>
agent-sessions verify [--config PATH] [--root PATH] <source-ref>
```

すべての操作は、安定した `v1` の JSON オブジェクトを一つ、標準出力に出力します。\
利用側は、ほかの応答フィールドを解釈する前に `schema_version` を確認しなければなりません。

本番用の登録一覧には、`codex`、`claude`、`chatgpt` のプロバイダーアダプターが含まれます。\
プロバイダーのアプリケーションを起動したり永続状態を書き込んだりせず、プロバイダーが保持する記録を直接検出します。

`list` の `--source-instance` と `--root` はプロバイダー内の選択に使うため、`--provider` の指定が必要です。
`list` がプロバイダーの環境変数または既定のルートを使う場合、暗黙に選ばれたルートが存在しなくてもソースは増えず、登録済みのほかのプロバイダーの結果は妨げられません。\
明示したルートと設定したルートは指定どおりに扱い、アクセスに失敗した場合はそれを報告します。

[CLI JSON 仕様](docs/cli-json-contract.md) に、フィールド、ページ分割、上限、終了コード、完全性、秘匿化の動作を記載しています。

## 基本的な使い方

利用可能な暗黙のルートからソースを検出するか、プロバイダーを明示して選択します。

```sh
agent-sessions list
agent-sessions list --provider codex
agent-sessions list --provider claude
agent-sessions list --provider chatgpt --source-instance chatgpt-personal --root /fictional/chatgpt-export.zip
```

`list` の応答から `identity.source_ref` を読み取り、そのままソースに対する操作へ渡します。

```sh
source_ref='as0:provider:source-instance:fingerprint-from-list'
agent-sessions show "$source_ref"
agent-sessions events "$source_ref"
agent-sessions verify "$source_ref"
```

`events` では `--limit` と、直前のページが返した不透明な `--cursor` を指定できます。\
標準以外のルートや複数の保存場所は、`--root`、`--source-instance`、または後述するマシン固有の設定で選択できます。

## 目標

`agent-sessions` は、各利用側がプロバイダーごとの保存形式やアクセス方法を理解しなくても、セッション履歴を再利用できるようにします。

中核となる目標は次のとおりです。

- 複数のプロバイダーと保存場所から、コーディングエージェントのセッションを検出する。
- セッションのメタデータとイベントを読み取り、正規化する。
- 後から確認できる出所の証拠が必要な場合に、ソースのバージョンを検証する。
- 同じプロバイダーの独立した複数のインスタンスに対応する。
- 厳格な読み取り専用境界を維持する。
- 既定では過去の実行状態に依存しない。
- 下流のツールに適した、上限付きの機械可読出力を提供する。

## 中核となる保証

### 読み取り専用

プロバイダーが保持する対話記録が元データです。\
読み取り操作では、プロバイダーの履歴、アプリケーションのメタデータ、利用側の状態を書き換えません。\
プロバイダーのファイルを読み取る際に、ホストのファイルシステムがアクセス時刻のメタデータを更新することはあります。

この読み取り専用の要件を満たす場合は、プロバイダーの公式インターフェースを優先します。\
十分に厳格な読み取り専用の経路がない場合は、より安全で対応済みのアクセス経路を使用できます。

### 既定ではステートレス

`agent-sessions` は次を所有しません。

- 処理済みセッションの登録簿
- 利用側のレビュー状態
- 正式な対話記録のデータベース
- 永続的な全文索引
- バックグラウンドの取り込み処理

同じソースデータと設定に対する読み取り操作の意味上の結果は、過去に `agent-sessions` を実行したかどうかに依存してはなりません。

削除可能なキャッシュを追加できるのは、測定により実質的な性能上の必要性が示された場合だけです。\
キャッシュを削除しても変わるのは性能だけであり、正しさには影響してはなりません。

### バックグラウンド動作なし

アーキテクチャは、デーモン、監視処理、タイマー、定期的な同期処理を必要としません。\
`agent-sessions` が動作していない間は、CPU、メモリ、ディスクの入出力を消費しません。\
読み取り専用コマンドは、`agent-sessions` のデータを永続的に書き込みません。

### プロバイダーが管理する生データ

対話記録全体を `agent-sessions` の保存領域にコピーしません。

プロバイダーがソースを削除または変更した後も証拠を残す必要がある利用側は、自分の用途に必要な最小限の派生した証拠を保持します。

## 対応プロバイダー

対応するプロバイダーは次のとおりです。

- Codex: [Codex プロバイダー](docs/codex.md) に記載した互換性境界で実装済み。
- Claude Code: [Claude Code プロバイダー](docs/claude.md) に記載した互換性境界で実装済み。
- ChatGPT Data Export: [ChatGPT Data Export プロバイダー](docs/chatgpt.md) に記載した互換性境界で、明示的に指定したエクスポート ZIP を実装済み。

## ソースモデル

1 台のコンピューター上で、一つのプロバイダーが複数の独立した保存場所を公開することがあります。

例えば、次のようになります。

```text
codex-wsl
codex-windows-app
claude-wsl
```

この境界を名前付きソースインスタンスとしてモデル化します。

論理ソースはそのインスタンス内で識別し、プロバイダー固有のセッション ID が全体で一意だとは仮定しません。

概念上は次の構成です。

```text
provider
source instance
provider-native source ID
```

ファイルシステムのパスは場所を示すものであり、論理識別子ではありません。

[アーキテクチャ](docs/architecture.md) に詳細なモデルを記載しています。

## 設定

一般的な単一ソース環境は、`agent-sessions` の設定なしで動作します。

ChatGPT Data Export はプロバイダーのホームではなく、ユーザーが選択したスナップショットなので、意図的に明示指定だけを受け付けます。\
`--provider chatgpt --root PATH` の両方を指定するか、ZIP のパスを `root` に持つ名前付き `chatgpt` ソースインスタンスを設定します。

明示的な設定は、次のような場合に使います。

- 一つのプロバイダーに複数のホームまたは保存場所がある場合。
- 標準ではないプロバイダーのホーム。
- マウントしたプロバイダーのデータ。

解決順序は次のとおりです。

```text
明示的な CLI オプション
        ↓
agent-sessions のユーザー設定
        ↓
プロバイダーの環境変数
        ↓
プロバイダーの既定値
```

設定は、可能な限りプロバイダーレベルのホームまたは同等のソース境界を識別します。\
プロバイダー内部のセッションディレクトリに関する知識はプロバイダーアダプターに属し、通常のユーザー設定には置きません。

任意の設定ファイルは、Go の `os.UserConfigDir` が返す OS ユーザー設定ディレクトリ配下の `agent-sessions/config.json` に置く厳密な JSON です。

```json
{
  "schema_version": "v1",
  "sources": [
    {
      "id": "codex-default",
      "provider": "codex",
      "root": "/fictional/provider-root"
    }
  ]
}
```

ソースルートは絶対形式のマシンローカルパスでなければなりません。\
この例は説明用であり、設定したソースを利用できるのは、そのプロバイダーアダプターが存在する場合だけです。

## 利用側との境界

`agent-sessions` は、ソースが処理済みであることの意味を定義しません。\
異なる利用側が、同じソースバージョンを異なる目的で独立に確認できます。

例えば、次のようになります。

```text
source X @ version A

利用側 1: 確認済み
利用側 2: 未確認
```

永続的な出所情報が必要な利用側は、次を保持できます。

- 論理ソース参照。
- 検証済みソースバージョンまたは内容ハッシュ。
- 上限付きの派生証拠。
- 自分の確認または判断の状態。

これらの利用側の判断は `agent-sessions` の状態にはなりません。

## セキュリティとプライバシー

過去の対話記録は、信頼できない入力です。

`agent-sessions` は次を行ってはなりません。

- 過去のセッションにあるコマンドを実行する。
- 過去のプロンプトやモデル出力を現在の指示として扱う。
- 記録に現れるという理由だけで、過去の URL やツール要求に従う。
- 秘密情報を不必要に公開する。
- 不完全な観測を完全なものとして黙って提示する。
- 調査中にプロバイダーが所有する履歴を変更する。

リポジトリのフィクスチャは合成データでなければならず、実際のセッション内容、非公開リポジトリ情報、認証情報、ホスト名、ユーザー名、マシン固有のパスを含めてはなりません。

## 対象外

`agent-sessions` は次を提供することを目的としません。

- エージェントハーネスの振り返りまたは改善。
- 個人メモリまたはドメイン固有の分類。
- 正式なトランスクリプトアーカイブ。
- 継続的なテレメトリ収集。
- クラウド同期。
- マシン間の履歴の自動同期。
- Web UI。
- 汎用チャットクライアント。
- ベクトル検索または意味検索用の索引を既定で提供すること。

これらの機能は下流の利用側または別のシステムに属します。

## ドキュメント

- [日本語参考訳と言語方針](docs/localization.md)
- [この README の日本語参考訳](README.ja.md)
- [アーキテクチャ](docs/architecture.md)
- [Codex プロバイダー](docs/codex.md)
- [Claude Code プロバイダー](docs/claude.md)
- [ChatGPT Data Export プロバイダー](docs/chatgpt.md)
- [ADR 0001: ステートレスで読み取り専用のソースアクセスを使う](docs/decisions/0001-use-stateless-read-only-source-access.md)
- [ADR 0002: プロバイダーのソースを名前付きインスタンスとしてモデル化する](docs/decisions/0002-model-provider-sources-as-named-instances.md)
- [ADR 0003: プロバイダーの検出とバージョン依存のデコードを分離する](docs/decisions/0003-separate-provider-discovery-from-version-sensitive-decoding.md)
- [CLI JSON 仕様](docs/cli-json-contract.md)

## 開発時の検証

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
