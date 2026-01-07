# ragujuary

Gemini File Search Stores を管理するためのCLIツール - Googleのフルマネージド RAG（Retrieval-Augmented Generation）システム。

## 機能

- Gemini File Search Stores の作成・管理
- 複数ディレクトリからのファイルアップロード（自動チャンキング・埋め込み生成）
- 自然言語によるドキュメント検索（RAG）
- 並列アップロード（デフォルト5ワーカー）
- チェックサムによる重複排除（変更のないファイルはスキップ）
- チェックサムを customMetadata に保存（マルチマシン同期対応）
- ファイルまたはストア全体の削除
- アップロードされたドキュメントのフィルタリング表示
- ローカルメタデータとリモート状態の同期
- リモートメタデータの取得（マルチマシン対応）
- 検証可能なレスポンスのための引用機能
- **MCP サーバー**: AIアシスタント（Claude Desktop、Cline等）に全機能を公開

## Gemini File Search とは？

Gemini File Search は、Gemini API に組み込まれたフルマネージドの RAG システムです。基本的な File API（48時間でファイルが期限切れ）とは異なり、File Search Stores は：

- 手動で削除するまでドキュメントを無期限に保存
- ドキュメントを自動的にチャンク分割し、埋め込みを作成
- コンテンツに対するセマンティック検索を提供
- 幅広いフォーマットをサポート（PDF、DOCX、TXT、JSON、コードファイルなど）
- 検証のためのレスポンスに引用を含む

## インストール

```bash
go install github.com/takeshy/ragujuary@latest
```

または、ソースからビルド：

```bash
git clone https://github.com/takeshy/ragujuary.git
cd ragujuary
go build -o ragujuary .
```

## 設定

Gemini API キーを設定：

```bash
export GEMINI_API_KEY=your-api-key
```

または、各コマンドで `--api-key` フラグを使用。

オプションで、デフォルトのストア名を設定：

```bash
export RAGUJUARY_STORE=mystore
```

または、各コマンドで `--store` / `-s` フラグを使用。

### ストア名の指定方法

ストアは **display name**（推奨）または完全なAPI名で指定できます：

```bash
# display name を使用（シンプル、推奨）
ragujuary list -s my-store --remote

# 完全なAPI名を使用（fileSearchStores/ プレフィックス付き）
ragujuary list -s fileSearchStores/mystore-abc123xyz --remote
```

利用可能なストアとdisplay nameを確認：

```bash
ragujuary list --stores
```

## 使い方

### ストアを作成してファイルをアップロード

```bash
# ストアを作成してファイルをアップロード
ragujuary upload --create -s mystore ./docs

# 複数ディレクトリからアップロード
ragujuary upload --create -s mystore ./docs ./src ./config

# パターンにマッチするファイルを除外
ragujuary upload --create -s mystore -e '\.git' -e 'node_modules' ./project

# 並列数を設定
ragujuary upload -s mystore -p 10 ./large-project

# ドライラン（アップロードされるファイルを確認）
ragujuary upload -s mystore --dry-run ./docs
```

### ドキュメントを検索（RAG）

```bash
# 基本的なクエリ
ragujuary query -s mystore "主な機能は何ですか？"

# 複数ストアを検索
ragujuary query --stores store1,store2 "全ドキュメントを横断検索"

# 別のモデルを使用（デフォルト: gemini-3-flash-preview）
ragujuary query -s mystore -m gemini-2.5-flash "アーキテクチャを説明して"

# 引用の詳細を表示
ragujuary query -s mystore --citations "認証はどのように機能しますか？"
```

### ストアとファイルを一覧表示

```bash
# すべての File Search Stores を表示
ragujuary list --stores

# ストア内のドキュメントを表示（リモートAPIから取得）
ragujuary list -s mystore --remote

# ローカルキャッシュからドキュメントを表示
ragujuary list -s mystore

# パターンでフィルタリング
ragujuary list -s mystore -P '\.go$'

# 詳細情報を表示
ragujuary list -s mystore -l --remote
```

### ファイルまたはストアを削除

```bash
# パターンにマッチするファイルを削除
ragujuary delete -s mystore -P '\.tmp$'

# 確認なしで強制削除
ragujuary delete -s mystore -P '\.log$' -f

# IDを指定してドキュメントを削除（重複削除に便利）
ragujuary delete -s mystore --id hometakeshyworkjoinshubotdo-mckqpvve11hv
ragujuary delete -s mystore --id doc-id-1 --id doc-id-2

# ストア全体を削除
ragujuary delete -s mystore --all

# 確認なしでストアを強制削除
ragujuary delete -s mystore --all -f
```

### ステータス

ファイルの状態を確認（変更済み、未変更、欠落）：

```bash
ragujuary status -s mystore
```

### 同期

ローカルメタデータをリモート状態と同期。リモートのドキュメントをローカルキャッシュにインポートします：

```bash
# リモートのドキュメントをローカルキャッシュにインポート
ragujuary sync -s mystore

# sync後はローカルキャッシュから一覧表示可能（高速、API呼び出し不要）
ragujuary list -s mystore
```

sync コマンドの動作：
- ローカルに存在しないリモートドキュメントをインポート
- リモートに存在しなくなった孤立エントリを削除
- ローカルエントリを現在のリモートドキュメントIDで更新

### フェッチ

リモートのドキュメントメタデータをローカルキャッシュに取得。複数マシン間の同期や、MCP経由でアップロードされたドキュメントのインポートに便利：

```bash
# リモートメタデータをローカルキャッシュに取得
ragujuary fetch -s mystore

# ローカルファイルのチェックサムが異なっても強制更新
ragujuary fetch -s mystore -f
```

fetch コマンドの動作：
- リモートストアから全ドキュメントのメタデータを取得（ファイル本体はダウンロードしない）
- ローカルファイルのチェックサムとリモートのチェックサム（customMetadataに保存）を比較
- チェックサムが一致すればローカルキャッシュを更新
- チェックサムが異なれば警告を表示してスキップ（`--force` でオーバーライド）
- ディスクにファイルがない場合は警告付きで処理

**複数マシンで使用する場合の注意**: 別のマシンからアップロードする前に、必ず `fetch` を実行してローカルキャッシュをリモートと同期してください。これにより重複ドキュメントの作成を防げます。

### クリーン

ローカルに存在しなくなったリモートドキュメントを削除：

```bash
ragujuary clean -s mystore
ragujuary clean -s mystore -f  # 確認なしで強制実行
```

### MCP サーバー

MCP（Model Context Protocol）サーバーを起動し、ragujuary の機能を Claude Desktop、Cline などの AI アシスタントに公開します。

#### トランスポートオプション

- **http**（推奨）: 双方向通信用の Streamable HTTP
- **sse**: HTTP 経由の Server-Sent Events（リモート接続用）
- **stdio**（デフォルト）: ローカル CLI 連携用

#### 使用方法

```bash
# HTTP サーバーをポート 8080 で起動（リモートアクセス推奨）
ragujuary serve --transport http --port 8080 --serve-api-key mysecretkey

# または環境変数で API キーを設定
export RAGUJUARY_SERVE_API_KEY=mysecretkey
ragujuary serve --transport http --port 8080

# SSE サーバーを起動（代替）
ragujuary serve --transport sse --port 8080 --serve-api-key mysecretkey

# stdio サーバーを起動（Claude Desktop ローカル連携用）
ragujuary serve
```

#### Claude Desktop 設定

`~/.config/claude/claude_desktop_config.json` に追加：

```json
{
  "mcpServers": {
    "ragujuary": {
      "command": "/path/to/ragujuary",
      "args": ["serve"],
      "env": {
        "GEMINI_API_KEY": "your-gemini-api-key"
      }
    }
  }
}
```

#### 利用可能な MCP ツール

MCP サーバーは 7 つのツールを公開しています。

##### `upload` - ファイルをストアにアップロード

単一ファイルを Gemini File Search Store にアップロードします。ファイルの内容を直接渡します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | File Search Store の名前 |
| `file_name` | string | はい | アップロードするファイル名またはパス |
| `file_content` | string | はい | ファイルの内容（プレーンテキストまたは base64 エンコード） |
| `is_base64` | boolean | いいえ | file_content が base64 エンコードの場合は true（PDF、画像などのバイナリファイル用） |

例（テキストファイル）:
```json
{
  "store_name": "my-docs",
  "file_name": "README.md",
  "file_content": "# My Document\n\nThis is the content."
}
```

例（バイナリファイル）:
```json
{
  "store_name": "my-docs",
  "file_name": "document.pdf",
  "file_content": "JVBERi0xLjQK...",
  "is_base64": true
}
```

##### `query` - ドキュメントを検索（RAG）

自然言語でセマンティック検索を実行します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | いいえ* | File Search Store の名前 |
| `store_names` | array | いいえ* | 複数の File Search Store の名前 |
| `question` | string | はい | ドキュメントに対する質問 |
| `model` | string | いいえ | 使用するモデル（デフォルト: gemini-3-flash-preview） |
| `metadata_filter` | string | いいえ | メタデータフィルタ式 |
| `show_citations` | boolean | いいえ | 引用の詳細を含める |

*`store_name` または `store_names` のいずれかが必要です。

例（単一ストア）:
```json
{
  "store_name": "my-docs",
  "question": "認証システムはどのように機能しますか？",
  "model": "gemini-2.5-flash",
  "show_citations": true
}
```

例（複数ストア）:
```json
{
  "store_names": ["docs-store", "api-store"],
  "question": "全ドキュメントを横断検索"
}
```

##### `list` - ストア内のドキュメントを一覧表示

File Search Store 内のドキュメントをフィルタリングして一覧表示します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | ストアの名前 |
| `pattern` | string | いいえ | 結果をフィルタする正規表現パターン |

例:
```json
{
  "store_name": "my-docs",
  "pattern": "\\.go$"
}
```

##### `delete` - ファイルを削除

ファイル名を指定してストアからファイルを削除します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | ストアの名前 |
| `file_name` | string | はい | 削除するファイル名 |

例:
```json
{
  "store_name": "my-docs",
  "file_name": "README.md"
}
```

##### `create_store` - ストアを作成

新しい File Search Store を作成します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | 新しいストアの表示名 |

例:
```json
{
  "store_name": "my-new-store"
}
```

##### `delete_store` - ストアを削除

File Search Store とそのすべてのドキュメントを削除します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | 削除するストアの名前 |

例:
```json
{
  "store_name": "my-docs"
}
```

##### `list_stores` - ストア一覧を取得

利用可能なすべての File Search Store を一覧表示します。

パラメータは不要です。

例:
```json
{}
```

#### HTTP 認証

HTTP/SSE トランスポートの認証設定：
- `--serve-api-key` フラグ
- `RAGUJUARY_SERVE_API_KEY` 環境変数

クライアントの認証方法：
- `X-API-Key` ヘッダー
- `Authorization: Bearer <key>` ヘッダー
- `api_key` クエリパラメータ

## データ保存

ファイルメタデータはデフォルトで `~/.ragujuary.json` に保存されます。`--data-file` で別の場所を指定可能。

各ストアで追跡される情報：
- ローカルファイルパス
- リモートドキュメントID
- SHA256 チェックサム
- ファイルサイズ
- アップロード日時
- MIME タイプ

## グローバルフラグ

| フラグ | 短縮形 | 説明 | デフォルト |
|------|-------|-------------|---------|
| `--api-key` | `-k` | Gemini API キー | `$GEMINI_API_KEY` |
| `--store` | `-s` | ストア名 | `$RAGUJUARY_STORE` または `default` |
| `--data-file` | `-d` | データファイルのパス | `~/.ragujuary.json` |
| `--parallelism` | `-p` | 並列アップロード数 | `5` |

## サポートされるファイル形式

File Search は幅広いフォーマットをサポート：
- ドキュメント: PDF, DOCX, TXT, MD
- データ: JSON, CSV, XML
- コード: Go, Python, JavaScript, TypeScript, Java, C, C++ など

## 料金

- インデックス作成時の埋め込み生成: 100万トークンあたり $0.15
- ストレージ: 無料
- クエリ時の埋め込み: 無料
- 取得トークン: 標準のコンテキストトークン料金

## 制限

- 最大ファイルサイズ: ファイルあたり 100 MB
- ストレージ: 1 GB（無料枠）〜 1 TB（Tier 3）
- プロジェクトあたりの最大ストア数: 10

## ライセンス

MIT
