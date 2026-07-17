# uec-portal-mcp

UEC 学生ポータルの掲示板記事を取得する MCP サーバーです。MCP クライアントから記事の検索・一覧取得と、記事本文・メタデータの取得を実行できます。

## Features

- UEC 学生ポータルへのログイン（ユーザー名・パスワード・TOTP）
- 掲示板記事の一覧取得、ページング、キーワード検索、公開年フィルター
- 記事 ID による本文・メタデータ取得
- MCP stdio transport
- MCP Streamable HTTP transport
- 認証セッションライブラリによる 30 分ごとの自動再認証
- Docker と mise による実行

## MCP tools

### `list_articles`

UEC 学生ポータルの掲示板記事の見出しを取得します。入力には次の項目を指定できます。

| 項目 | 説明 |
| --- | --- |
| `page` | 1 始まりのページ番号。省略時は 1 |
| `keyword` | 記事を検索する文字列 |
| `year` | 公開年による絞り込み。例: `2026` |

結果には `article_id`、タイトル、著者、カテゴリ、既読状態、公開日時が含まれます。

### `get_article`

`list_articles` で得られた `article_id` を指定して、記事本文とメタデータを取得します。

- `article_id`: 取得する記事 ID
- `history`: 検索・履歴結果に含まれる記事 ID の場合は `true`

## Authentication

実行時に次の環境変数を設定してください。3 つすべてが必須です。

```bash
export PORTAL_USERNAME="your-username"
export PORTAL_PASSWORD="your-password"
export PORTAL_OTP_SECRET="your-totp-secret"
```

`.env` ファイルを置いた場合は、Docker の mise タスクから自動的に読み込まれます。認証情報はリポジトリにコミットしないでください。

認証処理は `pkg/uecauth` パッケージとして分離されています。返される `Session` は `http.Client` と同様に `Do` を実装し、認証成功から 30 分が経過した後の最初のリクエストで、新しい Cookie Jar を使って自動的に再認証します。ポータル側の画面遷移状態を保つため、各HTTPリクエストは直列化され、直前のレスポンスヘッダーを受信するまで次のリクエストを送信しません。

```go
credentials := &uecauth.Credentials{
    Username:  username,
    Password:  password,
    OTPSecret: otpSecret,
}
authenticator, err := uecauth.New(credentials)
if err != nil {
    return err
}
session := authenticator.NewSession()

portalClient := portal.NewPortalClient(session)
```

## Requirements

- Go 1.26 以降
- または Docker と mise

## Usage

### stdio

```bash
go run . serve --stdin
```

### Streamable HTTP

`--http` を指定するか、モードを省略すると HTTP モードで起動します。

```bash
go run . serve --http
go run . serve --http --port 18080
```

ポートは `PORT` 環境変数が設定されていればそれを使用し、未設定の場合は `--port` の値を使用します。デフォルトは `8080` です。`--stdin` と `--http` は同時に指定できません。

詳細なログを有効にするには、グローバルオプション `--verbose`（または `-v`）を使用します。

```bash
go run . --verbose serve --stdin
```

## Docker

Docker 用の補助タスクは Make ではなく mise で提供しています。

```bash
mise run docker:build
mise run docker:run
mise run docker:stop
mise run docker:rm
```

`docker:run` はコンテナをバックグラウンドで起動し、`unless-stopped` の再起動ポリシーを設定します。主な環境変数は次のとおりです。

| 環境変数 | デフォルト | 説明 |
| --- | --- | --- |
| `IMAGE_NAME` | `uec-portal-mcp` | Docker イメージ名 |
| `CONTAINER_NAME` | `uec-portal-mcp` | コンテナ名 |
| `PORT` | `8080` | ホスト・コンテナのポートの基準値 |
| `HOST_PORT` | `PORT` | ホスト側の公開ポート |
| `CONTAINER_PORT` | `PORT` | コンテナ側の待受ポート |

例:

```bash
PORT=18080 CONTAINER_NAME=uec-portal-mcp-dev mise run docker:run
```

Dockerfile は非 root の distroless イメージを使用し、HTTP モードで起動します。

## Project structure

```text
.
├── cmd/                 # Cobra CLI と MCP の起動モード
├── pkg/uecauth/         # UEC認証と自動セッション更新
├── pkg/portal/          # 記事取得とHTMLパース
├── pkg/tools/           # MCP tool 定義とハンドラー
├── mise-tasks/docker/   # Docker 用 mise タスク
├── Dockerfile
└── main.go
```

## Development

テストを実行します。

```bash
go test ./...
```

実際のMCPサーバープロセスをstdio・HTTPの両方で起動し、UECポータルへのログインから記事取得まで確認するE2Eテストは、認証用環境変数を設定して次のように実行します。

```bash
go test -tags=e2e -run '^TestE2E' -v .
```

ビルドします。

```bash
go build -o uec-portal-mcp .
```

## License

未定義です。
