import { useCallback, useEffect, useState } from "react";
import { api, ListData, Status, Todo } from "./api";
import MyLists from "./MyLists";
import { addMyList } from "./storage";

const STATUSES: { key: Status; label: string }[] = [
  { key: "todo", label: "仕掛かり前" },
  { key: "doing", label: "仕掛かり中" },
  { key: "done", label: "完了" },
];
type Filter = Status | "all";

export default function App() {
  if (location.pathname === "/lists") return <MyLists />;
  const m = location.pathname.match(/^\/l\/([0-9a-f]{20})$/);
  return m ? <ListView slug={m[1]} /> : <Landing />;
}

function Landing() {
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const create = async () => {
    setBusy(true);
    try {
      const { slug } = await api.createList(title);
      addMyList(slug); // 作成した端末の一覧に記録
      location.href = `/l/${slug}`;
    } catch {
      setBusy(false);
      alert("作成に失敗しました。もう一度お試しください。");
    }
  };
  return (
    <main className="landing">
      <h1>みんなのTODO</h1>
      <p>ログイン不要。リストを作って、URLを送れば一緒に進められます。</p>
      <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="リスト名（例：引っ越し準備）" maxLength={100} />
      <button className="primary" onClick={create} disabled={busy}>リストを作る</button>
      <p className="notice" role="note">URLを知っていれば誰でも見られます。機密情報は書かないでください。</p>
      <a href="/lists">作ったリストを見る</a>
    </main>
  );
}

function ListView({ slug }: { slug: string }) {
  const [data, setData] = useState<ListData | null>(null);
  const [error, setError] = useState(false);
  const [filter, setFilter] = useState<Filter>("all");
  const [text, setText] = useState("");
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    try {
      setData(await api.getList(slug));
      setError(false);
    } catch {
      setError(true);
    }
  }, [slug]);

  // 3秒ごとに最新状態を取得（他の人の追加・更新を反映）。非表示タブでは停止。
  useEffect(() => {
    load();
    const t = setInterval(() => { if (!document.hidden) load(); }, 3000);
    const onVis = () => { if (!document.hidden) load(); };
    document.addEventListener("visibilitychange", onVis);
    return () => { clearInterval(t); document.removeEventListener("visibilitychange", onVis); };
  }, [load]);

  useEffect(() => { if (data) document.title = `${data.title} - みんなのTODO`; }, [data?.title]);

  const patch = (fn: (t: Todo[]) => Todo[]) => setData((d) => (d ? { ...d, todos: fn(d.todos) } : d));

  const setStatus = async (id: number, status: Status) => {
    patch((ts) => ts.map((t) => (t.id === id ? { ...t, status } : t)));
    try { await api.setStatus(slug, id, status); } catch { load(); }
  };
  const remove = async (id: number) => {
    if (!confirm("このタスクを削除しますか？")) return;
    patch((ts) => ts.filter((t) => t.id !== id));
    try { await api.remove(slug, id); } catch { load(); }
  };
  const add = async () => {
    const title = text.trim();
    if (!title) return;
    setText("");
    try {
      const t = await api.add(slug, title);
      patch((ts) => (ts.some((x) => x.id === t.id) ? ts : [...ts, t]));
    } catch { setText(title); alert("追加に失敗しました。"); }
  };
  const share = async () => {
    const url = location.href;
    if (navigator.share) {
      try { await navigator.share({ title: data?.title, url }); return; } catch { /* キャンセル時はコピーにフォールバック */ }
    }
    await navigator.clipboard.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  if (error && !data) return <main className="landing"><h1>見つかりません</h1><p>URLが正しいか確認してください。</p><a href="/">新しくリストを作る</a></main>;
  if (!data) return <main className="landing"><p>読み込み中…</p></main>;

  const count = (s: Status) => data.todos.filter((t) => t.status === s).length;
  const shown = filter === "all" ? data.todos : data.todos.filter((t) => t.status === filter);

  return (
    <div className="app">
      <header>
        <a className="back" href="/lists" aria-label="作ったリスト一覧へ">‹</a>
        <h1>{data.title}</h1>
        <button className="share" onClick={share}>{copied ? "コピーしました" : "共有"}</button>
      </header>
      <nav className="tabs">
        <button className={filter === "all" ? "on" : ""} onClick={() => setFilter("all")}>すべて {data.todos.length}</button>
        {STATUSES.map((s) => (
          <button key={s.key} className={filter === s.key ? "on" : ""} onClick={() => setFilter(s.key)}>{s.label} {count(s.key)}</button>
        ))}
      </nav>
      {error && <div className="offline">接続を確認しています…</div>}
      <ul className="list">
        {shown.length === 0 && <li className="empty">{data.todos.length === 0 ? "下の欄から最初のタスクを追加しましょう。" : "この状態のタスクはありません。"}</li>}
        {shown.map((t) => (
          <li key={t.id} className={`item s-${t.status}`}>
            <div className="title">
              <span>{t.title}</span>
              <button className="del" aria-label="削除" onClick={() => remove(t.id)}>×</button>
            </div>
            <div className="seg" role="group" aria-label="状態">
              {STATUSES.map((s) => (
                <button key={s.key} className={t.status === s.key ? `on c-${s.key}` : ""} aria-pressed={t.status === s.key} onClick={() => setStatus(t.id, s.key)}>{s.label}</button>
              ))}
            </div>
          </li>
        ))}
      </ul>
      <form className="adder" onSubmit={(e) => { e.preventDefault(); add(); }}>
        <input value={text} onChange={(e) => setText(e.target.value)} placeholder="タスクを追加" maxLength={200} enterKeyHint="send" />
        <button className="primary" type="submit" disabled={!text.trim()}>追加</button>
      </form>
    </div>
  );
}
