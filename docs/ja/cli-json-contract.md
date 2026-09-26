# CLI JSON 仕様

> **注記:** [英語版](../cli-json-contract.md) が正式版です。
> この日本語版は参考訳です。
> 内容に差異がある場合は英語版を優先します。

## 互換性の状態

公開レスポンススキーマは安定版 `v2` である。\
同じ仕様を、実運用の Codex、Claude Code、ChatGPT Data Export アダプターが実装する。

`list` の最小で完全なレスポンスは次のとおりである。

```json
{
  "schema_version": "v2",
  "cli_version": "dev",
  "operation": "list",
  "status": "complete",
  "data": {
    "sources": []
  },
  "page": {
    "limit": 50,
    "has_more": false
  },
  "omissions": []
}
```

自動処理用の仕様は JSON だけであり、人間向けの出力は実装していない。

エンベロープの各フィールドには、次の出現規則がある。

| フィールド | 出現規則 |
| --- | --- |
| `schema_version` | 必須。この仕様では常に `v2`。 |
| `cli_version` | 必須。実行ファイルのビルドを識別し、`dev` の場合がある。 |
| `operation` | 必須。`list`、`show`、`events`、`verify`、またはコマンドレベルエラー用の `cli` のいずれか。 |
| `status` | 必須。以下の完全性状態のいずれか。 |
| `data` | `complete` と `partial` の操作結果では必須。`unsupported` と `error` では存在しない。 |
| `page` | 成功または一部完了した `list` と `events` の結果では必須。それ以外では存在しない。 |
| `omissions` | 必須。`complete` では空、`partial` と `unsupported` では空でなく、`error` では出力処理による省略を記録する場合がある。 |
| `error` | `error` の場合だけ必須。それ以外の状態では存在しない。 |

### 互換性規則

利用者は、`data` やその他のバージョン依存フィールドを解釈する前に、`schema_version` を読み取り検証しなければならない。\
この仕様を実装する利用者は `v2` を受け付け、未知のメジャーバージョンを拒否する。

追加の任意フィールドは `v2` の中で導入でき、利用者はスキーマのメジャーバージョンを受け入れた後、未知のレスポンスフィールドを無視しなければならない。\
必須フィールドの削除や変更、既存フィールドの意味の変更、状態、イベント種別、終了コード、論理識別子の規則、その他の必須意味の非互換な変更には、新しいメジャースキーマバージョンが必要である。

`cli_version` は実行ファイルのバージョンを示し、レスポンススキーマを選択するものではない。\
設定スキーマは別に検証され、現在は `v1` だけを受け付ける。

## 操作

### `list`

`list` は `data.sources` と `page` オブジェクトを返す。\
ソースは、オフセットによるページ分割を適用する前に論理ソース参照で並べ替える。

各ソースには `identity`、`kind`、`relationships`、`metadata` が必要である。\
`version_hint`、`last_interaction_at`、`identity.provider_native_source_id` は任意であり、その他の識別フィールドはすべて必須である。

### `show <source-ref>`

`show` は `data.source` として、正規化した一つのソースを返す。

### `events <source-ref>`

`events` は `data.source_ref`、順序付けられた `data.events`、`page` オブジェクトを返す。\
イベントのインデックスは 0 始まりで、プロバイダーの順序を保つ。

### `verify <source-ref>`

`verify` は `data.source_ref` と `data.verified_version` を返す。

検証済みバージョンは、ドメイン区切り文字と名前順に並べた証拠チャンクから計算する SHA-256 である。\
各チャンクの名前と内容には、符号なし 64 ビット・ビッグエンディアンのバイト長を前置し、重複するチャンク名は拒否する。

`verified_version` には `algorithm`、`basis`、`value` が必要である。\
現在の値はそれぞれ `sha256`、`provider-content-v0`、`sha256:` を先頭に付けたダイジェストである。

### 操作データとページオブジェクト

`data` オブジェクトには、要求された操作に対応するフィールドだけを含める。

| 操作 | `data` の必須フィールド |
| --- | --- |
| `list` | `sources`。空でもよいソースオブジェクトの配列。 |
| `show` | `source`。一つのソースオブジェクト。 |
| `events` | `source_ref` と `events`。空でもよい配列。 |
| `verify` | `source_ref` と `verified_version`。 |

`page` オブジェクトには整数の `limit` と Boolean の `has_more` が必要である。\
`has_more` が true の場合は `next_cursor` が必要で、それ以外では存在しない。

各省略には、トークン値の `code` と `scope`、安全な人間向けの `message` が必要である。\
任意の正の整数 `count` は、その省略が表す既知の項目数を示す。\
これがないことは、影響を受けるプロバイダー値が 0 件であることを意味しない。

各メタデータエントリには、トークン値の `name` と安全な文字列 `value` が必要である。\
メタデータ名によって、プロバイダー固有の値にプロバイダーに依存しない意味を与えてはならない。

各関係には `kind` と `source_ref` が必要である。\
現在出力する関係種別は `parent` と `forked_from` で、どちらも明示されたプロバイダー所有の識別フィールドだけに基づく。\
安全に識別できない関係は、パスやイベント順から導出せず、アダプターが省略する。

ソースの `kind` は拡張可能なトークンである。\
現在出力する値は、Codex と Claude Code の記録では `session`、ChatGPT Data Export の会話では `conversation` である。\
利用者は、ソースが認識していない `kind` トークンを使っているというだけで、その他は有効な `v2` レスポンスを拒否してはならない。

存在する場合、`version_hint` には `kind` と `value` が必要で、`kind` も拡張可能である。\
継承履歴のない Codex ソースと Claude Code は安価な変更検出のために `stat_hash` を出力する。\
継承履歴を安全に解決できた Codex ソースは、採用したロールアウトの範囲に対する `content_hash` を出力する。\
ChatGPT Data Export はエクスポート ZIP 全体の SHA-256 識別子として `snapshot_hash` を出力する。\
利用者は認識済みのヒントを変更検出に使ってよく、認識していないヒント種別は無視しなければならない。

`last_interaction_at` がある場合、その値は末尾が `Z` の正規化された UTC の RFC 3339 時刻である。\
一つの論理ソースに記録されたユーザーまたはアシスタントのメッセージ、ツール呼び出し、ツール結果のうち、最も遅い時刻を表す。\
管理用の行、システム通知、ファイルの更新時刻は含めない。\
やり取りがない場合や、活動行の時刻の欠落・不正値、やり取りの可能性がある未知の行、プロバイダーの時刻用互換性境界に含まれない版、曖昧な成果物、継承履歴を解決できないことによって最終時刻を安全に確定できない場合、このフィールドはない。\
その場合は `scope: source` の `source_time_unavailable` を追加し、`list` または `show` を `partial` にする。\
`count` で影響するソース数をまとめて示す場合がある。\
ChatGPT Data Export では選択中の会話経路にあるやり取りを対象とし、別の分岐によってこのフィールドは進まない。

一定期間内の対象を選ぶ利用者は、`list` の全ページを取得し、`last_interaction_at` がないソースを時刻が分かる対象から除外し、絶対日時として比較した後、時刻の降順と `identity.source_ref` の昇順で並べる。\
ソースの時刻が欠けている場合や未取得のページがある場合など、`partial` の一覧からは対象の全件を確定できない。

## 論理識別子

ソース識別子は、次の情報を分離する。

```text
provider
source instance
provider-native source ID
```

コマンドラインのソース参照は、次の形式である。

```text
as0:<provider>:<source-instance>:<source-id-fingerprint>
```

フィンガープリントは、プロバイダー固有のソース ID をドメイン分離して SHA-256 ハッシュ化したものである。\
そのため、この参照は生のネイティブ ID やファイルシステムの場所を埋め込まずに決定的なままである。

`identity` オブジェクトには `provider`、`source_instance`、`provider_source_fingerprint`、`source_ref` が必要である。\
任意の `provider_native_source_id` には、出力上限内でプロバイダーが所有する論理 ID を含める。\
生の値を省略した場合でも、フィンガープリントとソース参照は利用できる。

`as0` プレフィックスとソースフィンガープリントの `v0` ドメイン区切り文字は、`schema_version` とは独立してバージョン管理する。\
公開レスポンススキーマが `v2` に達したというだけで、これらを変更することはない。

## イベント

各イベントには `index`、`kind`、種類に対応する本文一つ、サイズ制限内の `metadata` が必要である。

イベントは `time_state` と、確認できた場合に正規の UTC RFC 3339 形式の `recorded_at` を返す。\
日時はプロバイダーの記録に属し、操作の開始や終了を推測したものではない。\
`time_state` は `available`、`absent`、`unavailable`、`unsupported` のいずれかで、`available` の場合だけ `recorded_at` を含める。\
日時がない場合や利用できない場合は `event_time_omitted` を追加し、`events` を `partial` にする。

対応する種類は次のとおりである。

- `message`: `role`（`user` または `assistant`）、記録された `text`、真偽値の `truncated`。
- `tool_call`: 任意の `call_id` と記録された `name`、必須の `category` と `input_state`、任意の `action` と `input`。
- `tool_result`: 必須の `outcome`、`correlation_state`、`content_state`、任意の `call_id`、`exit_code`、`content`。
- `error`: `category` と、記録されたプロバイダーエラーの `message`。

### 記録された内容

`input` と `content` は共通の表現を使う。

```json
{"format":"json","text":"[\"go\",\"test\",\"./...\"]","truncated":false}
```

`format` は `text` または `json`、`text` は常に文字列、`truncated` は必須の真偽値である。\
任意の真偽値 `omitted` は、読める内容を残しながら非テキスト部分や未対応の部分を除外したことを示す。\
JSON 本文は `truncated` が真でなければ有効な JSON であり、切り詰めた先頭部分は解析できないことがある。\
JSON の整形やオブジェクトの項目順について元のバイト列の保持は保証しないが、制限内で文字列の値と配列の順序を保持する。\
引数の配列は JSON 文字列内の配列として保持し、実行可能なコマンドへ連結しない。

`input_state` と `content_state` は `available`、`absent`、`unavailable`、`unsupported` を使う。\
`available` は対応する内容オブジェクトを必須とし、空文字列、空のオブジェクト、空の配列も含む。\
それ以外の状態では内容オブジェクトを省略する。\
`absent` は確認済みの形式で不在と分かること、`unavailable` は期待する情報の欠落や読み取り不能、`unsupported` は未対応の形式を表す。\
`unavailable` と `unsupported` では `tool_input_unavailable` または `tool_content_unavailable` の省略理由を追加する。

道具の名前と JSON の入力・結果の項目名は製品固有の値であり、共通の道具の仕様ではない。\
記録された名前がない場合は項目を省略し、`tool_name_unavailable` を報告する。\
`category` は大まかな分類を維持し、`action` がある場合は `execute`、`read`、`search`、`write`、`edit`、`invoke` のいずれかである。

### 成否と対応付け

`outcome` は `success`、`failure`、`unknown` のいずれかである。\
成功・失敗は、意味を確認したプロバイダーの明示的な項目だけで確定し、`PASS` やエラーらしい文章では判定しない。\
`unknown` でも読める本文を保持し、`tool_outcome_unknown` を追加する。

`correlation_state` は `matched`、`unmatched`、`ambiguous` のいずれかである。\
`matched` では、選択した論理履歴内の明示的な相関識別子により、呼び出しと結果を一対一に確定できることを必須とし、この状態だけ結果に `call_id` を含める。\
結果が呼び出しより先に記録されていたり、別のページに現れたりしてもよい。\
`unmatched` は対応する呼び出しを確定できないこと、`ambiguous` は呼び出しまたは結果の識別子の重複で一意に対応できないことを表す。\
どちらも本文を保持して結果の `call_id` を省略し、`correlation_omitted` を追加する。\
識別子がない、または重複した呼び出しは `call_id` なしで保持し、結果の記録がない呼び出しも省略理由を報告する。\
隣接する順序や同じ文章から関係を推測しない。

対応付けはページ分割の前に、選択した履歴全体で行う。\
正規化した呼び出し ID は応答の出力形式とは独立した、アダプターが所有する `v0` のアルゴリズム名前空間を維持し、生のプロバイダー相関識別子を公開する呼び出し ID にはしない。

## プロバイダー固有の情報

プロバイダー固有の成果物の配置、行の種類、相関値は、所有するアダプターの内部に残す。\
安全に公開でき、プロバイダーに依存しない意味を作り出さずに済むサイズ制限内のメタデータを、共通の `metadata` 配列に含めてもよい。

アダプターは、忠実に表現できない重要な値を省略として報告する。\
不安定なプロバイダーフィールドを安定したスキーマへ昇格させたり、同等の共通の意味を推測して `complete` としたりしない。

## 完全性と省略

| 状態 | 意味 |
| --- | --- |
| `complete` | 重要な値が省略、切り詰め、未対応、不明のいずれにもなっていない。 |
| `partial` | 有用な出力はあるが、少なくとも一つの省略を報告している。 |
| `unsupported` | プロバイダーまたは操作が仕様を安全に満たせない。 |
| `error` | 操作が失敗し、構造化エラーを含む。 |

`complete` のレスポンスには、省略もエラーも含められない。\
`partial` のレスポンスには少なくとも一つの省略が含まれる。\
未知のプロバイダー形式や、出力の安全性を確認できない値は `complete` にしてはならない。

`list` では、複数のアダプターの結果をアダプターの順序とは独立して集約する。\
観測したすべてのアダプター結果が `unsupported` の場合は `unsupported` とし、`unsupported` の省略に `complete` または `partial` の観測を組み合わせた場合は `partial` とする。

`has_more: true` のページ分割は、`partial`、コード `pagination` の省略、バージョン付きの `next_cursor` を返す。\
カーソルの内容は利用者にとって不透明である。

現在のカーソルは `v0:` で始まる。\
このカーソル形式のバージョンは `schema_version` から独立している。

## 構造化エラーと終了コード

構造化エラーには、トークン値の `code` と `category`、固定の人間向けの `message`、Boolean の `retryable`、`details` 配列が必要である。\
各詳細エントリには、トークン値の `name` と安全な文字列 `value` が必要で、配列は空でもよい。\
現在の CLI エラーは再試行可能ではない。

安定したエラーカテゴリは `usage`、`configuration`、`provider`、`resource`、`internal` である。\
現在のエラーコードは次のとおりである。

- `missing_command`、`unknown_command`、`invalid_arguments`、`invalid_pagination`、`invalid_cursor`、`invalid_source_ref`。
- `source_resolution_failed`、`config_path_unavailable`、`invalid_configuration`。
- `source_not_found`、`provider_failure`、`invalid_provider_result`、`invalid_archive`、`invalid_json`、`duplicate_archive_member`、`unsafe_archive_member`、`source_changed`。
- `provider_resource_limit`。
- `event_limit_exceeded`、`verification_failed`、`output_bound_exceeded`、`response_limit_exceeded`。
- `invalid_result`。

省略とエラーのコード集合は `v2` の中で増やせる。\
`schema_version` を受け入れた後、未知のコードに遭遇した利用者は、結果を `complete` と扱わず、宣言された未完了またはエラー状態を保持しなければならない。

| 終了コード | 意味 |
| --- | --- |
| `0` | `complete` または `partial` の結果。 |
| `2` | 無効なコマンド、フラグ、カーソル、ソース参照、設定。 |
| `3` | 未対応のプロバイダーまたは操作。 |
| `4` | プロバイダー、I/O、検証、リソース、エンコーディング、内部の失敗。 |

基盤となるエラー、ソースルート、生の引数は、公開診断へコピーしない。

## 設定

既定のパスは、`os.UserConfigDir` が返すプラットフォームのユーザー設定ディレクトリ下にある `agent-sessions/config.json` である。\
既定のファイルがないことは有効であり、ディレクトリやファイルの作成も引き起こさない。

設定は厳密な JSON である。

```json
{
  "schema_version": "v1",
  "sources": [
    {"id": "example-default", "provider": "example", "root": "/fictional/root"}
  ]
}
```

プロバイダー ID とソースインスタンス ID は `[a-z][a-z0-9-]{0,62}` に一致する。\
一つのプロバイダー内でソースインスタンス ID は一意であり、ルートは絶対パスで、未知のフィールドは拒否する。

`list --source-instance` と `list --root` のオプションには、ソースインスタンス ID とルートがプロバイダー単位で管理されるため、`--provider` が必要である。

解決の優先順位は次のとおりである。

1. `--root` によるコマンドライン上書き。
2. 一致するユーザー設定。
3. プロバイダーの環境変数。
4. プロバイダーの既定値。

`list` では、プロバイダーの環境変数または既定値だけで選ばれたルートがない場合、ソースインスタンスを追加しない。\
これにより、登録されたすべてのプロバイダーをインストールしなくても、プロバイダーに依存しない検出で利用可能なプロバイダーを返せる。\
明示的な `--root` または設定されたルートは権威ある指定なので、どちらかへのアクセス失敗は操作エラーとして残る。

環境変数や既定のルートを持たないプロバイダーは、暗黙のソースインスタンスを追加しない。\
`chatgpt` プロバイダーは明示指定だけを受け付ける。\
そのルートはユーザーが選択した正確な Data Export ZIP であり、検索対象のディレクトリではない。

`--config` は明示的な設定ファイルを選択する。\
そのファイルを作成または更新するものではない。

## リソース制限

| リソース | 制限 |
| --- | ---: |
| 設定ファイル | 1 MiB |
| バッファリングするプロバイダーまたは検証証拠 | 64 MiB |
| JSON の入れ子 | 64 階層 |
| ソース観測あたりのイベント | 100,000 |
| 動的出力文字列 | 64 KiB |
| 一つの発言・ツール入力・ツール結果の内容 | 64 KiB |
| ソースまたはイベントあたりのメタデータエントリ | 64 |
| ソースあたりの関係 | 64 |
| ページサイズ | 既定 50、最大 100 |
| エンコード済みレスポンス | 8 MiB |

共通の 64 MiB の証拠制限は、証拠チャンクを CLI に返すアダプターに適用する。\
アーカイブアダプターは、プロバイダーが定める制限内のより大きなソースを、証拠チャンクとして実体化せずに同じ `provider-content-v0` 検証済みバージョンアルゴリズムへストリーム処理してよい。\
プロバイダー固有の入力制限は各アダプターに記載し、8 MiB のレスポンス制限を増やすものではない。

切り詰めた動的出力には `resource_truncation` の省略を追加し、完全な状態のままにはできない。

自由形式の文字列は、有効な UTF-8 の境界でのみ切り詰める。\
サイズ超過した生のプロバイダー固有ソース ID は省略するが、フィンガープリントとソース参照は保持する。\
サイズ超過した構造上の識別子は、別の識別子へ切り詰めず、構造化されたリソースエラーとして安全側に失敗する。

## 内容の扱い

記録された内容は自動で隠さずに返し、認証情報らしい値、パス、接続先、コード、検索文字列、コマンドの文章を含むことがある。\
内容を隠す切替設定はなく、`redaction_policy_version` は出力しない。\
利用側が分析、保存、外部への共有に使う内容を決める。\
CLI の診断には、基になったエラーや元の記録を不必要にコピーしない。\
過去の内容は未検証のデータであり、実行したり現在の指示として扱ったりしない。

一つの内容全体を 64 KiB に制限し、複数の結果欄や文章ブロックがあっても上限を増やさない。\
内容を切り詰めた場合は `truncated` と `tool_input_truncated` または `tool_content_truncated` を報告し、応答全体も `partial` にする。\
共通の最終出力処理による切り詰めは `resource_truncation` を報告する。\
非テキスト部分や未対応の部分を除外した場合は `unsupported_content` などの省略理由を報告する。\
一つの本文の続きを取得する機能はない。

JSON エンコード後に応答が 8 MiB を超える場合は `response_limit_exceeded` を返し、部分的な JSON を出力しない。\
ページ分割する操作では、同じ取得位置で `--limit` を減らして再試行できる。\
同じ要求をそのまま再試行しても解決しないため、`retryable` は偽のままである。\
一つの本文自体の切り詰めは、件数を減らしても回復しない。\
元の記録が取得の間に変わると、位置に基づくページ分割では一定の記録の完全取得を保証できない。\
同じ元データと実行ファイルで取得し、必要に応じて前後の検証済みバージョンを比較する。

## v1 からの移行

v2 は自動の内容隠蔽の保証を削除し、入力、結果、成否不明、対応先不明を表す。\
`success`、`excerpt`、`redacted`、`evidence_state` は新しい項目へ置き換え、`input_state: withheld` は `available` と実際の `input` に置き換える。\
新しい実行ファイルに v1 の出力を選ぶ機能はない。

利用側は v2 の受け入れ、入力と結果の読み取り、不明と省略の扱い、保存と共有の範囲を確認してから、固定した実行ファイルを更新する。\
v1 が必要な場合や問題が出た場合は、以前の実行ファイルを利用できる。\
設定ファイルの `schema_version: v1`、コマンドと引数、ソース参照、検証用ハッシュの方式は変わらない。
