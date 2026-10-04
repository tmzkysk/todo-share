# みんなのTODO

ログイン不要・URL共有で複数人が同じTODOリストを更新できるアプリ。
Go + MySQL + React(TypeScript)。

## 起動 (Docker)
    docker compose up --build
    # http://localhost:8080

## ローカル開発
    # 1) MySQL 起動 (例: docker run -p 3306:3306 -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=todo mysql:8.4)
    cd backend && go mod tidy && go run .
    # 2) 別ターミナル
    cd frontend && npm install && npm run dev   # http://localhost:5173 (/api は :8080 にプロキシ)

## 仕組み
- リスト作成時に推測困難なランダムURL(`/l/<20桁hex>`)を発行。URLを知る人だけが閲覧・編集可能。
- 同期はポーリング(3秒間隔)。追加・ステータス変更は楽観的UI更新。
- API: POST /api/lists, GET /api/lists/{slug}, POST /api/lists/{slug}/todos, PATCH|DELETE /api/lists/{slug}/todos/{id}

## 公開 (Render + Aiven)
1. **Aiven**: 無料のMySQLサービスを作成。Overviewから `Host` / `Port` / `User` / `Password` / `Database`(通常 defaultdb) を控え、`CA Certificate` を ca.pem としてダウンロード。
2. **DSNを用意する**（AivenのService URI `mysql://...?ssl-mode=REQUIRED` をそのまま `DB_DSN` に入れてもよい。その場合も `DB_CA_PEM` は必須）。Go形式で書く場合は下記（Aivenは独自CAのため `tls=aiven` が必要）:
       avnadmin:<PASSWORD>@tcp(<HOST>:<PORT>)/defaultdb?parseTime=true&charset=utf8mb4&tls=aiven
3. **GitHub**: このディレクトリをリポジトリとしてpush。
4. **Render**: New > Blueprint でリポジトリを選択（`render.yaml` を使用）。
   環境変数 `DB_DSN` に 2 の文字列、`DB_CA_PEM` に ca.pem の中身（-----BEGIN〜END-----全体）を入力。
5. デプロイ完了後、発行された `https://xxx.onrender.com` を開く。テーブルは起動時に自動作成される。

無料枠の注意: Renderの無料Webサービスは15分間アクセスがないとスリープする（初回表示に時間がかかる）。
