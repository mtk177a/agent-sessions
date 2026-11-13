# Codex プロバイダー

> **注記:** [英語版](../provider-codex.md) が正式版である。
> この日本語版は参考訳である。
> 内容に差異がある場合は英語版を優先する。

この文書では、`codex` アダプターで保存済みのセッション記録を読み取る方法と、出力の読み方を説明する。\
アダプターは、Codex がセッション履歴を保存したファイル（ロールアウト）を、Codex App Server を起動せずに読み取る。

JSON 応答の共通フィールドは [CLI 仕様](cli-json-contract.md) を参照する。

## 利用手順

### 1. 記録の保存場所を選ぶ

読み取り対象には、Codex ホームを指定する。\
その下の `sessions` や `archived_sessions` は指定しない。

保存場所は、次の優先順位で選ぶ。

1. 明示的な `--root`。
2. 設定されたソースインスタンス。
3. `$CODEX_HOME`。
4. 公式資料に記載された既定値 `~/.codex`。

以下の方法から、保存場所の指定方法を選ぶ。

#### プロバイダーの設定を使う

Codex は `$CODEX_HOME` をデータディレクトリとして文書化しており、この変数が設定されていない場合は `~/.codex` を使用する。

明示的なルートや設定済みのインスタンスがなければ、`$CODEX_HOME` を使う。\
この環境変数が設定されていなければ、`~/.codex` を使う。

この場合は、ルートを指定せずにセッションを一覧にする。

```sh
agent-sessions list --provider codex
```

#### コマンドで指定する

`--root` で保存場所を指定し、セッションを一覧にする。

```sh
agent-sessions list --provider codex --root /fictional/codex-home
```

#### 設定ファイルで指定する

[設定ファイル](cli-json-contract.md#読み取り対象の設定) に、名前付きソースインスタンスを追加する。\
たとえば、次のように指定する。

```json
{
  "sources": [
    {
      "id": "codex-records",
      "provider": "codex",
      "root": "/fictional/codex-home"
    }
  ]
}
```

その後、設定済みのインスタンスにあるセッションを一覧にする。

```sh
agent-sessions list --provider codex --source-instance codex-records
```

### 2. セッションを読み取る

`list` の結果から、読みたいセッションの `identity.source_ref` をコピーする。\
下のコマンドの `<source_ref from list>` を、コピーした値でそのまま置き換える。\
`list` で `--root` を使った場合は、このコマンドや、後で同じセッションを指定するコマンドにも同じ値を渡す。

```sh
agent-sessions events --root /fictional/codex-home '<source_ref from list>'
```

プロバイダーの設定または設定済みのインスタンスを使う場合は、このコマンドの `--root` を省略する。

## 対応形式

保存済みの `legacy` と `paginated` の履歴から、`events` はメッセージなどのイベントを返し、`verify` は履歴の内容を照合するためのハッシュ値を返す。\
履歴に記録された Codex CLI のバージョンが [アダプターの互換性一覧](../../internal/provider/codex/compatibility.go) に載っていない場合も、次の条件で対応する。

| 操作 | 対応範囲 |
| --- | --- |
| `events` と `verify` | バージョンと履歴モードの組み合わせが一覧にあれば、確認済みの規則で読む。一覧にない場合は、記録された履歴モードから規則を選び、その規則で扱える行を読む。 |
| `list` と `show` の時刻取得 | 一覧にある組み合わせに対応する。`0.155.0` 以降も、バージョン番号の書式が正しく、読み取り対象のすべての行が既知の構造に合う場合は対応する。 |

履歴の順序と範囲を確定できる場合、`events` は読み取れるイベントを保存順に返し、認識できない行があったことを `omissions` で報告する。\
`verify` も認識できない行を `omissions` で報告するが、照合用のハッシュ値の計算には、その行のバイト列も含める。\
履歴の順序または引き継ぐ範囲を確定できない場合は、`events` と `verify` のどちらも `unsupported` を返す。

`events` が一部のイベントを返せる場合でも、未知の行や不正な時刻があると最後のやり取りの時刻を確定できないことがある。

**圧縮ロールアウト**

圧縮された `.jsonl.zst` ロールアウトは認識するが、実行ファイルに外部圧縮依存がないためデコードしない。

## 出力の読み方

### 取得したデータ

操作に応じて、`data` からセッションの情報、イベント、または内容の照合に使う値を取得できる。

#### ソース情報

セッションの情報は、`list` では `data.sources`、`show` では `data.source` に入る。\
最後のやり取りの時刻を確定できたセッションでは、`last_interaction_at` でその時刻を確認できる。

#### イベント

`events` の `data.events` には、対応するメッセージ、ツール呼び出し、ツール結果、プロバイダーのエラーが入る。

ツール入力やツール結果は、1 件ごとに全体を 64 KiB 以内に収める。\
内容を切り詰めたり省いたりした場合、結果は不完全になる。

#### 検証済みバージョン

`verify` は、選択した論理履歴を照合するためのハッシュ値を `data.verified_version` に含めて返す。

### 処理の状態

#### 状態を示す status

`status` で、読み取った内容を結果に含められた範囲と、処理が失敗したかどうかを確認する。

| 条件 | 状態 |
| --- | --- |
| 観測した内容を省略なく表現できる。 | `complete` |
| 省略はあるが、有用な観測結果を得られる。 | `partial` |
| ソースは認識できるが、安全で有用な結果を得られない。 | `unsupported` |
| I/O またはリソース処理が失敗する。 | `error` |

#### 省略理由を示す omissions

次の条件がある場合、結果は `complete` にならない。

- 不明なエンベロープやイベント種別がある。
- 既知だが未対応の項目やメッセージ内容がある。
- JSONL が不正、または行がサイズ上限を超えている。
- 対応関係やツール結果が欠けている、または呼び出し ID が重複している。
- 関係識別子が不正である。
- 要求された操作が、その Codex のバージョンに対応していない。

公開イベント形式で表せない既知の項目は、引き続き明示的な省略として報告する。

#### 失敗の原因を示す error

I/O やリソース処理などの失敗は、構造化された `error` で説明する。\
フィールドとコードは、共通の [エラー仕様](cli-json-contract.md#エラー) を参照する。

## 入力の上限

Codex アダプターの入力には、次の上限を設ける。

| 入力 | 上限 |
| --- | ---: |
| 論理履歴の合計 | 128 MiB |
| 履歴の 1 行 | 4 MiB |
| ロールアウト | 32 個 |
| ヘッダー | 1 MiB |
| 検出するファイル | 100,000 個 |
| 正規化するイベント | 100,000 件 |

これらは、公開する [応答とページ分割の上限](cli-json-contract.md#リソース制限) に加わるアダプター入力の上限である。

## 出力の取り扱い

記録された内容は、自動で隠蔽せずに返す。\
内容には秘密の値が含まれることがある。\
保存する内容と外部への共有は呼び出し側が決める。\
[記録内容の取り扱い](cli-json-contract.md#記録内容の取り扱い) を参照する。

## 記録の読み取りと出力の組み立て

ロールアウトの JSONL は、Codex のバージョンに依存するアダプター入力であり、安定した公開 API ではない。\
アダプターの読み取り規則を変更するときは、互換性や入力の安全性について [共通の保守ルール](architecture.md#保守) に従う。

### 記録の読み取り

#### ロールアウトの検出

セッションのトランスクリプトは `sessions` の下に保存され、アーカイブ済みトランスクリプトは `archived_sessions` の下に保存される。

アダプターは、この 2 か所を決まった順序で調べる。\
プロバイダーのルートに含まれるサイズ制限内の通常ファイルだけを受け付け、シンボリックリンクと特殊ファイルを拒否し、内容を開くときにファイルとルートの同一性を再確認する。

#### 保存形式の判定

**対応形式**

アダプターは legacy、native paginated、canonicalized legacy paginated の履歴を区別する。\
[アダプターの互換性一覧](../../internal/provider/codex/compatibility.go) は、確認したバージョンと履歴モードの組み合わせごとに読み取り規則を定める。

canonicalized legacy paginated の読み取り規則は、旧形式からページ分割形式へ移行した履歴で確認した。\
対象となる履歴のヘッダーに記録された CLI バージョンは、`0.92.0`、`0.94.0`、`0.94.0-alpha.10`、`0.95.0-alpha.3`、`0.98.0`、`0.99.0-alpha.5`、`0.101.0`、`0.117.0`、`0.118.0`、`0.139.0`、`0.142.5`、`0.144.2` である。

Codex の [旧形式からページ分割形式への移行処理](https://github.com/openai/codex/blob/94174e44cbc54cece45f6052328ca0c2cd7a8a2a/codex-rs/thread-store/src/local/rollout_migration/canonicalizer.rs) は、保存行を書き換えるとき、ヘッダーに記録された CLI バージョンを保持する。\
確認済みの移行履歴では、旧形式の応答行からツール呼び出しと結果を取得し、完了済み項目から同じ実行を重ねて出力することを防ぐ。

**構造の検査**

一覧にない組み合わせで `events` と `verify` を実行するときは、記録された履歴モード（`history_mode`）が `legacy` か未指定なら legacy の読み取り規則を使い、`paginated` なら native paginated の読み取り規則を使う。\
読み取り対象の各行について、その規則が認識する行の種類と必須の JSON フィールドを確認する。\
フィールドの追加は許容する。

一覧にない `paginated` の履歴と、確認済みの移行履歴では、各行に記録された `ordinal` の値が連続していなければならない。\
canonicalized legacy paginated 形式では、移行履歴で確認した行の種類も検査する。

### 論理履歴の構成

安全に解決できた継承履歴と切り取り履歴は、`list`、`show`、`events`、`verify` で一貫して使用する。

#### 現在のロールアウトの選択

同じスレッドのロールアウトファイルが複数あるときは、Codex と同じ規則で、ファイル名の時刻が最も新しいファイルを選ぶ。\
同じ時刻の候補が複数あれば、ロールアウト ID を小文字に揃えて辞書順で比較し、最後のものを選ぶ。\
その候補と時刻もロールアウト ID も同じ別ファイルがある場合は、読み取り対象を確定できないため、`events` と `verify` は `unsupported` を返す。

#### 継承部分の解決

`history_base` の参照先のロールアウト ID をたどり、記録された行番号とバイト位置までの継承部分だけを読む。

#### 子セッションの履歴境界

`subagent_history_start_ordinal` で、親からコピーされた行を除外し、選択中の子セッション自身の行だけを残す。

`subagent_history_start_ordinal` より前の行は親からコピーされた文脈なので、子ソースのヒント、イベント、検証、やり取り時刻に影響しない。

### ソース情報の生成

#### 識別情報

識別子の検出では、サイズ制限内の最初の JSONL レコードを読み取る。

`session_meta.payload.id` フィールドはプロバイダー固有のスレッド識別子なので、ロールアウトファイルをアクティブな場所とアーカイブ済みの場所の間で移動しても `source_ref` は変わらない。

#### バージョンヒント

`version_hint` は、記録が変わった可能性を調べるための値である。\
内容の検証には `verify` が返す `verified_version` を使う。

**ヒントの算出**

選択した履歴によって、ヒントとハッシュ化する内容を次のように決める。

| 履歴の条件 | ヒントとハッシュ化する内容 |
| --- | --- |
| 継承や切り取りがない。 | `stat_hash`: 現在のファイルサイズと変更時刻を、ドメイン分離してハッシュ化する。 |
| 継承履歴またはサブエージェントの履歴境界を安全に解決できる。 | `content_hash`: Codex のヒント用 `v3` 名前空間を使う。選択中のヘッダーと、行を採用する各継承元ロールアウトのヘッダー・採用したバイト範囲を、論理的な順序でハッシュ化する。 |

**履歴を解決できない場合**

履歴を解決できない場合、ヒントは出さない。

**アルゴリズムの名前空間**

現在のファイルのサイズと変更時刻に基づくヒントは、Codex の `v0` アルゴリズム名前空間を使う。\
ヒントの `v0` と `v3` の名前空間は、公開応答スキーマから独立している。

#### ソース間の関係

親子関係とフォーク関係は、明示された `parent_thread_id` と `forked_from_id` のメタデータからのみ出力する。

#### 最後のやり取りの時刻

認識したメッセージ行とツール行の時刻のうち、最も遅いものを `last_interaction_at` とする。

`list` と `show` は、やり取りの時刻を確定するため、現在有効な履歴を上限内で逐次読み取る。

この時刻の対象は、[論理履歴](#論理履歴の構成) に含まれる記録だけである。

##### 対応バージョンと行構造の条件

やり取りの時刻は、`0.155.0` 以降も、バージョン番号の書式が正しく、読み取り対象のすべてのロールアウトが既知の `legacy` または native `paginated` の構造に合う場合に読み取る。\
この条件では、バージョンの上限を設けない。\
新しいバージョンの paginated 記録では、`ordinal` の値が連続し、読み取り対象のすべての行が、既知のメッセージ、ツール、応答、ライフサイクル、管理用の形式のいずれかに合う必要がある。\
やり取りとして時刻を進める既知のイベント行と応答行は、既知の構造で必須となる最上位フィールドと JSON 型を維持しなければならないが、フィールドの追加は許容する。

##### 記録の種類ごとの時刻判定

記録の種類ごとの時刻への影響を次に示す。\
いずれも、前述の履歴形式、バージョン、行構造の条件を満たす必要がある。

| 記録 | ソースの時刻への影響 |
| --- | --- |
| 完了した `FileChange` と `CollabAgentToolCall` | ツール操作として時刻を進める。 |
| `FunctionCallOutput` | ツール結果として時刻を進める。 |
| `SubAgentActivity` | 子エージェントの状態通知なので、時刻を進めない。 |
| 完了した `Extension` のうち、確認済みの `web.search` または `clock.sleep` | ツール操作として時刻を進める。 |
| それ以外の `Extension` の種類 | やり取りかどうか未確定のため、ソースの時刻を出さない。 |
| 完了した `WebSearch`、確認済みの生の `web_search_call` と `tool_search_call`、`tool_search_output` の結果 | 時刻を進める。生の行と完了行が同じ操作を表す場合も、その中で最も遅い時刻を使う。 |
| `0.144.2` の完了した `Sleep` | ツール操作として時刻を進める。 |
| Canonicalized legacy paginated 履歴の完了した `Reasoning`、`Plan`、`ContextCompaction` と `compacted` 行 | 時刻を進めない。 |
| `0.139.0`、`0.142.5`、`0.144.2` で確認済みの `world_state` スナップショット | 管理用の記録なので、時刻を進めない。 |
| `0.142.5` の `token_usage_record` | 利用量の記録なので、時刻を進めない。 |
| `0.144.2` の `inter_agent_communication_metadata` | 管理用の記録なので、時刻を進めない。 |
| それより後のトークン使用量、ライフサイクルイベント、プロバイダーエラー | 時刻を進めない。 |

Canonicalized legacy paginated ロールアウトでは、観測済みのメッセージ、ツール、検索、管理用の行だけを分類する。

##### 時刻を確定できない条件

未知の行や種類、必須の管理用フィールドの不正、既知の構造を維持しない将来の変更がある場合は、ソースの時刻を出さない。

時刻の欠落・不正値、不明確な行、履歴を解決できない場合、入力上限を超えた場合は、ファイルの更新時刻で補わず、ソースの時刻を出さない。

#### 記録された作業場所

`list` と `show` は `session_meta.payload.cwd`、`turn_context.payload.cwd`、完了した `CommandExecution.cwd`、記録された従来の `local_shell_call.action.cwd` を読む。\
コマンド文字列を解析せず、共通の `session`、`turn`、`tool` の根拠にする。\
作業ディレクトリを取得する際は、やり取りの時刻と同じ対応条件を使い、新しいバージョンについても行構造を検査する。

`CommandExecution.cwd` は `PathUri` 形式で記録される。\
アダプターはローカルの `file:` URI を実行環境のパス形式に従う絶対パスへ変換し、絶対パスで記録された値も受け付ける。\
リモートホストを指す URI、通常のパスに変換できない代替 URI、別のプラットフォームのパスがある場合は、作業場所の取得範囲を partial とする。

選択したヘッダーと有効な論理履歴の行から場所を取得する。\
子セッションの境界より前の親のコピーと、継承対象から外れた後続部分は含めない。\
場所の欠落・不正、未知の行、解決できない履歴、リソース上限がある場合は、作業場所の取得範囲を partial とする。\
履歴境界の検査が失敗した場合は、その走査で得た観測を捨てる。\
Codex の Git URL、ブランチ、コミットではローカル clone を識別しない。\
削除済み worktree で使える保存済みの対応がない場合、リポジトリ検索は残存する Git 登録を使うことができ、それもなければ所属は不明のままとする。

公式の [永続化プロトコルの構造](https://github.com/openai/codex/blob/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/protocol.rs) が、セッションとターンの `cwd` および Git 情報を定義する。\
公式の [item の構造](https://github.com/openai/codex/blob/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/items.rs) が、コマンドの作業場所を定義する。\
公式の [`PathUri` の実装](https://github.com/openai/codex/blob/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/utils/path-uri/src/lib.rs) が、作業場所の URI への変換方法を定義する。\
これらのアダプター入力は安定した公開フィールドではない。

### イベントの生成

#### 対象行の選択

アダプターは、公開される観測に必要な `session_meta`、`event_msg`、`response_item` のロールアウト用エンベロープを認識する。

公開イベントの基準とする行は、履歴の形式によって異なる。

| 形式 | 公開イベントの基準とする行 |
| --- | --- |
| Legacy | `user_message` と `agent_message` がメッセージの基準となる。応答のツール呼び出しと保存済みの出力行を、対応関係の根拠とする。 |
| Native paginated | 完了済みの `UserMessage`、`AgentMessage`、`CommandExecution`、`McpToolCall`、`DynamicToolCall` を基準とする。下位レベルの応答行は重ねて出力しない。 |
| Canonicalized legacy paginated | 完了済みメッセージ項目を基準とする。ツール呼び出しと結果の対応には旧形式の応答行を使い、完了済みツール項目は重ねて出力しない。 |

互換性一覧にないバージョンと履歴モードの組み合わせで `paginated` の履歴を読む場合、`events` は、legacy の読み取り規則でツール呼び出しや結果として正規化できる下位レベルの応答行を、1 行ずつ `omissions` で報告する。\
これらの行が、その履歴の完了済み項目と重複するかどうかは確定できない。

**公開イベントにしない行**

互換性一覧にないバージョンと履歴モードの組み合わせで `legacy` または `paginated` を読む場合、`events` は `web_search_call`、`tool_search_call`、`tool_search_output` の応答行を `unsupported_event` として報告する。\
`verify` は、選択した履歴の検証済みバージョンの算出に、これらの認識済みの行も含める。

`0.153.0` の `token_usage_record` は管理用の記録である。\
`realtime_item` が現れた場合は、やり取りとして正規化できないため、不完全として扱う。

#### 種別固有の変換

対応するイベントの種別ごとに、次の規則で変換する。

##### メッセージイベント

メッセージイベントは、前述の履歴形式ごとの基準行から生成する。

##### ツール呼び出しイベント

**カテゴリ**

公開カテゴリは `shell`、`file_change`、`mcp`、`tool` を維持する。

**名前**

ツール名は記録された `tool` を使い、コマンド項目は記録の種類名 `CommandExecution`、旧形式のローカル実行は `local_shell_call` を使う。

**操作**

コマンドは `execute`、MCP と動的なツールは `invoke`、旧形式の既知のパッチ呼び出しは `edit` を返す。

**入力**

旧形式の呼び出しは記録された `name` と `arguments` または `input` を返す。\
ローカル実行は記録された `action` オブジェクトを返し、配列のコマンドと他の入力項目を保持する。\
完了項目ではコマンドの `command` と、記録がある場合の `cwd` と `interaction_input`、MCP と動的なツールの `arguments` を返す。

想定される入力項目がない旧記録では、入力なしと推測せず `input_state: unavailable` を返す。

**呼び出し ID**

ツール呼び出し ID は、スレッド識別子と明示的なプロバイダー相関識別子から生成する、アダプターが所有する決定的なハッシュである。

**メタデータ**

MCP の `server` と動的なツールの `namespace` は、存在する場合に製品固有のメタデータとして保持する。

##### ツール結果イベント

**本文**

コマンドの結果は、記録された `aggregated_output`、`stdout`、`stderr`、`formatted_output` を保持する。\
欄が 1 つならテキストで返す。\
複数なら JSON にし、欄ごとに分けて返す。\
記録された空の本文も、本文なしとは区別する。

MCP の文章と `structuredContent` は両方保持し、動的なツールの `inputText` と旧形式の `input_text` の結果も返す。

複数の文章ブロックは順序を保った JSON 配列とし、非テキスト部分や未対応の部分は省略として報告する。\
既知のエラー欄は保持し、通常の出力と併存する場合は `output` と `error` を持つ JSON とする。

legacy と native paginated の履歴にある完了済みの `FunctionCallOutput` も本文を保持する。

**呼び出しとの対応**

対応付けは選択した論理履歴全体で行い、対応先がない、または識別子が重複した結果も本文を保持する。

完了済みの `FunctionCallOutput` は、項目 ID から呼び出しとの対応を確定できないため、独立した結果として扱う。

**成否**

成否はコマンドの `exit_code`、動的なツールの明示的な `success`、MCP の `isError`、確認済みの `status` から判定する。\
それらで確定できなければ `unknown` とし、本文の `PASS` などから推測しない。

旧形式の出力行には内部の成功値が保存されないため、本文を保持し、`outcome: unknown` を返す。

独立した `FunctionCallOutput` の成否も不明とする。

**メタデータ**

独立した `FunctionCallOutput` に記録された名前は、`tool_name` メタデータに保持する。

##### プロバイダーエラーイベント

エラーは、明示されたプロバイダーのエラーイベントから正規化する。

#### 日時フィールドの設定

各イベントは正規のロールアウト行の日時を使う。\
完了項目の呼び出しと結果は同じ日時であり、別々の開始・終了時刻を確定したものではない。

### 検証済みバージョンの算出

**検証する内容**

論理履歴の検証では、共通の `VerifiedVersion` 仕様を通じて、上限内のプロバイダー内容を逐次ハッシュ化する。

**検証証拠の表現**

切り取りのないロールアウトではパスを含まない証拠名 `rollout/primary.jsonl` を維持し、論理履歴では順序付きのヘッダー用・履歴用の名前を使う。

## 互換性の根拠

### 保存場所の根拠

OpenAI の [Codex トラブルシューティング文書](https://learn.chatgpt.com/ja-JP/docs/reference/troubleshooting) には、Codex のホームディレクトリ、アクティブなセッションのトランスクリプトの場所、アーカイブ済みセッションのトランスクリプトの場所が記載されている。\
JSONL の行のデコードは、保存された行が安定した公開 API であるという主張ではなく、アダプターが所有する互換性境界である。

### 内部形式の確認資料

**履歴形式の確認資料**

native paginated 形式の項目と応答の保存規則は、対応する公式実装の [`0.147.0`](https://github.com/openai/codex/blob/rust-v0.147.0/codex-rs/rollout/src/policy.rs)、[`0.148.0-alpha.9`](https://github.com/openai/codex/blob/rust-v0.148.0-alpha.9/codex-rs/rollout/src/policy.rs) などと、保存済みロールアウトの構造に対する上限付きの調査で確認した。\
元の [`0.98.0`](https://github.com/openai/codex/blob/rust-v0.98.0/codex-rs/protocol/src/protocol.rs) と [`0.117.0`](https://github.com/openai/codex/blob/rust-v0.117.0/codex-rs/protocol/src/protocol.rs) の形式には、ページ分割形式の行番号がなかった。

#### 確認したバージョン

**CLI のバージョン**

確認に使用した資料には、公式の `openai/codex` タグ [`rust-v0.149.1`](https://github.com/openai/codex/tree/rust-v0.149.1) のコミット [`ff29a44391deccde0aba0f8390337d7f3c319ea4`](https://github.com/openai/codex/commit/ff29a44391deccde0aba0f8390337d7f3c319ea4) がある。

`0.153.0` の境界は、公式の [`rust-v0.153.0` の履歴実装](https://github.com/openai/codex/blob/rust-v0.153.0/codex-rs/history/src/lib.rs) に対して確認した。

一覧にあるこれらのバージョンは、公式の [`0.152.0`](https://github.com/openai/codex/blob/rust-v0.152.0/codex-rs/protocol/src/items.rs)、[`0.153.3`](https://github.com/openai/codex/blob/rust-v0.153.3/codex-rs/protocol/src/items.rs)、[`0.153.4`](https://github.com/openai/codex/blob/rust-v0.153.4/codex-rs/protocol/src/items.rs)、[`0.154.0`](https://github.com/openai/codex/blob/rust-v0.154.0/codex-rs/protocol/src/items.rs) の項目定義と、保存済みロールアウトの構造に対する上限付き・読み取り専用の調査で確認した。\
非公開の本文や識別子は、このリポジトリへ複製していない。

**App のビルドバージョン**

Codex App のビルドバージョンは、公式の [`0.154.0-alpha.6.2`](https://github.com/openai/codex/blob/rust-v0.154.0-alpha.6.2/codex-rs/protocol/src/items.rs) と [`0.155.0-alpha.9.2`](https://raw.githubusercontent.com/openai/codex/4607249e430dac1c961df4dc615beae88e33cec8/codex-rs/protocol/src/items.rs) の項目定義、および保存済みロールアウトの構造に対して確認した。\
`0.155.0-alpha.9.2` では、観測された 2 種類の履歴モードを調べた。
