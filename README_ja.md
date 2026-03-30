# ragujuary

Google の Gemini API を使った RAG（Retrieval-Augmented Generation）のための CLI ツール & MCP サーバー。

## 機能

### 2つの RAG モード

**FileSearch モード**（マネージド RAG）:
- Gemini File Search Stores の作成・管理
- サーバーサイドでの自動チャンキング・埋め込み生成によるファイルアップロード
- 引用付き自然言語ドキュメント検索
- 並列アップロード（デフォルト5ワーカー）
- チェックサムによる重複排除（変更のないファイルはスキップ）
- チェックサムを customMetadata に保存（マルチマシン同期対応）
- sync/fetch によるマルチマシンワークフロー

**Embedding モード**（ローカル RAG）:
- Gemini Embedding API によるファイルインデックス（`gemini-embedding-2-preview`）
- **マルチモーダル対応**: 画像（PNG/JPEG）、PDF、動画（MP4）、音声（MP3/WAV）をテキストと共にインデックス
- ローカルベクトルストレージとコサイン類似度検索
- スマートテキストチャンキング（段落・文境界対応、日本語対応）
- 差分インデックス（変更されたファイルのみ再エンベディング）
- チャンクサイズ、オーバーラップ、top-K、最小スコアを設定可能
- OpenAI互換バックエンド（Ollama、LM Studio）対応（テキストのみ）

### 共通機能
- ファイルまたはストア全体の削除
- アップロード/インデックス済みドキュメントのフィルタリング表示
- **MCP サーバー**: AIアシスタント（Claude Desktop、Cline等）に全機能を公開

## Gemini File Search とは？

Gemini File Search は、Gemini API に組み込まれたフルマネージドの RAG システムです。基本的な File API（48時間でファイルが期限切れ）とは異なり、File Search Stores は：

- 手動で削除するまでドキュメントを無期限に保存
- ドキュメントを自動的にチャンク分割し、埋め込みを作成
- コンテンツに対するセマンティック検索を提供
- 幅広いフォーマットをサポート（PDF、DOCX、TXT、JSON、コードファイルなど）
- 検証のためのレスポンスに引用を含む

## Gemini Embedding とは？

Gemini Embedding API は、コンテンツのベクトル表現を統一セマンティック空間に生成し、クロスモーダル検索（テキストで画像を検索など）を可能にします：

- モデル: `gemini-embedding-2-preview`（マルチモーダル、8192トークン）
- **対応モダリティ**: テキスト、画像（PNG/JPEG）、PDF（6ページまで）、動画（120秒まで）、音声（80秒まで）
- 検索に最適化されたタスクタイプ: `RETRIEVAL_DOCUMENT`（インデックス時）、`RETRIEVAL_QUERY`（検索時）
- 出力次元数を設定可能（128-3072、デフォルト768）
- テキストはバッチ埋め込み、マルチモーダルは個別埋め込み

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

### FileSearch モード

#### ストアを作成してファイルをアップロード

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

#### ドキュメントを検索（RAG）

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

#### ストアとファイルを一覧表示

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

#### ファイルまたはストアを削除

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

#### ステータス

ファイルの状態を確認（変更済み、未変更、欠落）：

```bash
ragujuary status -s mystore
```

#### 同期

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

#### フェッチ

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

#### クリーン

ローカルに存在しなくなったリモートドキュメントを削除：

```bash
ragujuary clean -s mystore
ragujuary clean -s mystore -f  # 確認なしで強制実行
```

### Embedding モード

#### ファイルをインデックス

```bash
# ディレクトリからファイルをインデックス（テキストはチャンク分割、画像/PDF/動画/音声はそのまま埋め込み）
ragujuary embed index -s mystore ./docs

# 複数ディレクトリから除外パターン付きでインデックス
ragujuary embed index -s mystore -e '\.git' -e 'node_modules' ./project ./docs

# チャンキングパラメータをカスタマイズ（テキストファイルに適用）
ragujuary embed index -s mystore --chunk-size 500 --chunk-overlap 100 ./docs

# 別のモデル/次元数を使用
ragujuary embed index -s mystore --model gemini-embedding-2-preview --dimension 1536 ./docs

# Ollama を使用（テキストのみ、マルチモーダルファイルは警告付きでスキップ）
ragujuary embed index -s mystore --embed-url http://localhost:11434 --model nomic-embed-text ./docs
```

インデックスは差分更新：チェックサムが変更されたファイルのみ再エンベディングされます。
マルチモーダルファイル（画像、PDF、動画、音声）は拡張子で自動検出され、チャンク分割なしの単一ベクトルとして埋め込まれます。

#### エンベディングストアを検索

テキスト質問で全インデックスコンテンツ（テキストチャンク＋マルチモーダルファイル）を横断検索します（同一埋め込み空間でのクロスモーダル検索）。

```bash
# セマンティック検索（テキストとマルチモーダルコンテンツを横断検索）
ragujuary embed query -s mystore "認証はどのように機能しますか？"

# 画像を説明で検索
ragujuary embed query -s mystore "猫の写真"

# 結果をカスタマイズ
ragujuary embed query -s mystore --top-k 10 --min-score 0.5 "エラーハンドリングパターン"

# 外部ツールで作成された RAG インデックスを検索
ragujuary embed query --dir /path/to/external/rag/store "検索クエリ"
```

`--dir` フラグを使うと、他のツールで作成された RAG インデックスを検索できます。snake_case（ragujuary形式）と camelCase の両方の JSON フィールド名を自動検出します。`--dir` 指定時は `--store` は不要です。

#### インデックス済みファイルを一覧表示

```bash
# すべてのエンベディングストアを表示
ragujuary embed list --stores

# 特定のストア内のファイルを表示
ragujuary embed list -s mystore
```

#### インデックスからファイルを削除

```bash
# パターンにマッチするファイルを削除
ragujuary embed delete -s mystore -P '\.tmp$'
```

#### ストア全体をクリア

```bash
ragujuary embed clear -s mystore
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

# 特定のストアのみに制限（不要な API 呼び出しを削減）
ragujuary serve --stores mystore1,mystore2

# 単一ストアモード（全ツールで store_name が省略可能に）
ragujuary serve --stores mystore
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

MCP サーバーは 8 つの統合ツールを公開しています。各ツールはストア名からストアの種類（Embedding / FileSearch）を自動判別します。

##### `upload` - ファイルをアップロード/インデックス

ファイルをストアにアップロードします。Embedding ストアではローカルにインデックス、FileSearch ストアでは Gemini クラウドにアップロードします。マルチモーダルコンテンツ（画像/PDF/動画/音声）の場合は `mime_type` と `is_base64=true` を設定。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | ストアの名前 |
| `file_name` | string | はい | アップロードするファイル名またはパス |
| `file_content` | string | はい | ファイルの内容（プレーンテキストまたは base64 エンコード） |
| `is_base64` | boolean | いいえ | file_content が base64 エンコードの場合は true |
| `mime_type` | string | いいえ | バイナリコンテンツの MIME タイプ（Embedding ストアのみ） |
| `chunk_size` | integer | いいえ | チャンクサイズ（文字数、デフォルト: 1000、Embedding ストアのみ） |
| `chunk_overlap` | integer | いいえ | チャンクオーバーラップ（文字数、デフォルト: 200、Embedding ストアのみ） |
| `dimension` | integer | いいえ | エンベディング次元数（デフォルト: 768、Embedding ストアのみ） |

##### `query` - ドキュメントを検索

自然言語でドキュメントを検索します。Embedding ストアではコサイン類似度検索、FileSearch ストアでは Gemini の引用付き生成を使用します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | いいえ* | ストアの名前 |
| `store_names` | array | いいえ* | 複数のストアの名前 |
| `question` | string | はい | ドキュメントに対する質問 |
| `model` | string | いいえ | 使用するモデル（デフォルト: gemini-3-flash-preview、FileSearch のみ） |
| `metadata_filter` | string | いいえ | メタデータフィルタ式（FileSearch のみ） |
| `show_citations` | boolean | いいえ | 引用の詳細を含める（FileSearch のみ） |
| `top_k` | integer | いいえ | 上位結果数（デフォルト: 5、Embedding ストアのみ） |
| `min_score` | number | いいえ | 最小類似度スコア（デフォルト: 0.3、Embedding ストアのみ） |

*`store_name` または `store_names` のいずれかが必要です。

##### `list` - ストア内のドキュメントを一覧表示

ストア内のドキュメントをフィルタリングして一覧表示します。ストアの種類を自動判別します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | ストアの名前 |
| `pattern` | string | いいえ | 結果をフィルタする正規表現パターン |

##### `delete` - ファイルを削除

ファイル名を指定してストアからファイルを削除します。ストアの種類を自動判別します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | ストアの名前 |
| `file_name` | string | はい | 削除するファイル名 |

##### `create_store` - ストアを作成

新しいストアを作成します。`type` で Embedding と FileSearch を選択できます。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | 新しいストアの表示名 |
| `type` | string | いいえ | `embed` で Embedding ストア、`filesearch`（デフォルト）で FileSearch ストア |

##### `delete_store` - ストアを削除

ストアとそのすべてのデータを削除します。ストアの種類を自動判別します。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | 削除するストアの名前 |

##### `list_stores` - ストア一覧を取得

利用可能なすべてのストア（Embedding と FileSearch の両方）を一覧表示します。

パラメータは不要です。

##### `upload_directory` - ディレクトリからファイルをアップロード/インデックス

ディレクトリからファイルをストアにアップロード/インデックスします。ストアの種類を自動判別：Embedding ストアではローカルにインデックス、FileSearch ストアでは Gemini クラウドにアップロードします。再帰的にファイルを検出し、未変更のファイルをスキップします。

| パラメータ | 型 | 必須 | 説明 |
|-----------|------|------|-------------|
| `store_name` | string | はい | ストアの名前 |
| `directories` | array | はい | ディレクトリパスのリスト |
| `exclude_patterns` | array | いいえ | ファイルを除外する正規表現パターン |
| `parallelism` | integer | いいえ | 並列アップロード数（デフォルト: 5、FileSearch のみ） |
| `chunk_size` | integer | いいえ | チャンクサイズ（文字数、デフォルト: 1000、Embedding ストアのみ） |
| `chunk_overlap` | integer | いいえ | チャンクオーバーラップ（文字数、デフォルト: 200、Embedding ストアのみ） |
| `dimension` | integer | いいえ | エンベディング次元数（デフォルト: 768、Embedding ストアのみ） |

#### HTTP 認証

HTTP/SSE トランスポートの認証設定：
- `--serve-api-key` フラグ
- `RAGUJUARY_SERVE_API_KEY` 環境変数

クライアントの認証方法：
- `X-API-Key` ヘッダー
- `Authorization: Bearer <key>` ヘッダー
- `api_key` クエリパラメータ

## データ保存

### FileSearch モード
ファイルメタデータはデフォルトで `~/.ragujuary.json` に保存されます。`--data-file` で別の場所を指定可能。

各ストアで追跡される情報：
- ローカルファイルパス
- リモートドキュメントID
- SHA256 チェックサム
- ファイルサイズ
- アップロード日時
- MIME タイプ

### Embedding モード
エンベディングストアは `~/.ragujuary-embed/<ストア名>/` に保存されます：
- `index.json` - チャンクメタデータ、ファイルチェックサム、エンベディングモデル、次元数
- `vectors.bin` - Float32 ベクトルデータ（バイナリ）

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

### FileSearch モード
- インデックス作成時の埋め込み生成: 100万トークンあたり $0.15
- ストレージ: 無料
- クエリ時の埋め込み: 無料
- 取得トークン: 標準のコンテキストトークン料金

### Embedding モード
- 埋め込み生成: 100万トークンあたり $0.15
- ローカルストレージ: 無料（ディスク容量のみ）

## 制限

### FileSearch モード
- 最大ファイルサイズ: ファイルあたり 100 MB
- ストレージ: 1 GB（無料枠）〜 1 TB（Tier 3）
- プロジェクトあたりの最大ストア数: 10

### Embedding モード
- テキスト: チャンクあたり 8,192 トークン
- 画像: リクエストあたり最大 6 枚（PNG、JPEG）
- PDF: ファイルあたり最大 6 ページ
- 動画: 最大 120 秒（音声トラック付きは 80 秒）
- 音声: 最大 80 秒
- 出力次元数: 128〜3,072
- マルチモーダル埋め込みは Gemini バックエンド必須（OpenAI互換バックエンドでは利用不可）

## ライセンス

MIT
