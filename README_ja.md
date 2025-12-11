# ragujuary

Gemini File Search Stores を管理するためのCLIツール - Googleのフルマネージド RAG（Retrieval-Augmented Generation）システム。

## 機能

- Gemini File Search Stores の作成・管理
- 複数ディレクトリからのファイルアップロード（自動チャンキング・埋め込み生成）
- 自然言語によるドキュメント検索（RAG）
- 並列アップロード（デフォルト5ワーカー）
- チェックサムによる重複排除（変更のないファイルはスキップ）
- ファイルまたはストア全体の削除
- アップロードされたドキュメントのフィルタリング表示
- ローカルメタデータとリモート状態の同期
- 検証可能なレスポンスのための引用機能

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

# 別のモデルを使用
ragujuary query -s mystore -m gemini-2.5-pro "アーキテクチャを説明して"

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

# ストア全体を削除
ragujuary delete -s mystore --store

# 確認なしでストアを強制削除
ragujuary delete -s mystore --store -f
```

### ステータス

ファイルの状態を確認（変更済み、未変更、欠落）：

```bash
ragujuary status -s mystore
```

### 同期

ローカルメタデータをリモート状態と同期：

```bash
ragujuary sync -s mystore
```

### クリーン

ローカルに存在しなくなったリモートドキュメントを削除：

```bash
ragujuary clean -s mystore
ragujuary clean -s mystore -f  # 確認なしで強制実行
```

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
| `--store` | `-s` | ストア名 | `default` |
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
