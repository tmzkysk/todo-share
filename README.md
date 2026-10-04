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
