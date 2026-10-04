import { useCallback, useEffect, useState } from "react";
import { api, Status, Todo } from "./api";
import { loadMyLists, MyList, setRemoved } from "./storage";

const LABEL: Record<Status, string> = { todo: "仕掛かり前", doing: "仕掛かり中", done: "完了" };

// 仕掛かり中以上が1件でもあれば「仕掛かり中」、全件完了なら「完了」、それ以外は「仕掛かり前」
export function summarize(todos: Todo[]): Status {
  if (todos.length > 0 && todos.every((t) => t.status === "done")) return "done";
  if (todos.some((t) => t.status === "doing" || t.status === "done")) return "doing";
  return "todo";
}

type Info = { title: string; status: Status } | "missing" | "error";

export default function MyLists() {
  const [items, setItems] = useState<MyList[]>(() => loadMyLists());
  const [infos, setInfos] = useState<Record<string, Info>>({});
  const [loading, setLoading] = useState(true);
  const [showRemoved, setShowRemoved] = useState(false);

  const refresh = useCallback(async () => {
    const current = loadMyLists();
    setItems(current);
    const results = await Promise.all(
      current.map(async (i): Promise<[string, Info]> => {
        try {
          const d = await api.getList(i.slug);
          return [i.slug, { title: d.title, status: summarize(d.todos) }];
        } catch (e) {
          return [i.slug, (e as Error).message === "404" ? "missing" : "error"];
        }
      }),
    );
    // 通信エラー時は前回の表示を残す
    setInfos((prev) => {
      const next = { ...prev };
      for (const [slug, info] of results) if (info !== "error" || !next[slug]) next[slug] = info;
      return next;
    });
    setLoading(false);
  }, []);

  useEffect(() => {
    refresh();
    const onVis = () => { if (!document.hidden) refresh(); };
    document.addEventListener("visibilitychange", onVis);
    return () => document.removeEventListener("visibilitychange", onVis);
  }, [refresh]);

  const toggleRemoved = (slug: string, removed: boolean) => {
    setRemoved(slug, removed); // データは残るので確認なしで切り替え
    setItems(loadMyLists());
  };

  const sorted = items.filter((i) => showRemoved || !i.removed).sort((a, b) => b.createdAt.localeCompare(a.createdAt));

  return (
    <div className="app">
      <header>
        <h1>作ったリスト</h1>
        <a className="share" href="/">新規作成</a>
      </header>
      <p className="notice" role="note">
        <strong>ご注意：</strong>TODOリストはURLを知っていれば誰でも閲覧・編集できます。
        機密情報や個人情報は書かないでください。
      </p>
      <label className="check">
        <input type="checkbox" checked={showRemoved} onChange={(e) => setShowRemoved(e.target.checked)} />
        削除したリストも表示
      </label>
      <ul className="list">
        {sorted.length === 0 && (
          <li className="empty">
            {loading ? "読み込み中…" : items.length === 0 ? "まだリストがありません。「新規作成」から作ってみましょう。" : "表示するリストがありません。"}
          </li>
        )}
        {sorted.map((i) => {
          const info = infos[i.slug];
          const status = info && typeof info === "object" ? info.status : null;
          const date = new Date(i.createdAt).toLocaleDateString("ja-JP");
          return (
            <li key={i.slug} className={`item s-${status ?? "todo"}${i.removed ? " removed" : ""}`}>
              <div className="title">
                {info === "missing" ? <span>このリストは見つかりません</span>
                  : info === "error" || !info ? <span>{info ? "読み込めませんでした" : "読み込み中…"}</span>
                  : i.removed ? <span>{info.title}</span>
                  : <a className="row-link" href={`/l/${i.slug}`}>{info.title}</a>}
                {i.removed
                  ? <button className="restore" onClick={() => toggleRemoved(i.slug, false)}>元に戻す</button>
                  : <button className="del" aria-label="一覧から外す" onClick={() => toggleRemoved(i.slug, true)}>×</button>}
              </div>
              <div className="meta">
                <span>作成日 {date}{i.removed && " ・削除済み"}</span>
                {status && <span className={`badge c-${status}`}>{LABEL[status]}</span>}
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
