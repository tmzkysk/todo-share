// 自分が作ったリストのURL(slug)を端末のlocalStorageに保存する
// removed: 「一覧から外した」印。データは消さず、あとで元に戻せる
export interface MyList { slug: string; createdAt: string; removed?: boolean }

const KEY = "todo-share:my-lists";

export function loadMyLists(): MyList[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? "[]");
    return Array.isArray(v) ? v.filter((x) => x && typeof x.slug === "string" && typeof x.createdAt === "string") : [];
  } catch {
    return [];
  }
}

function save(items: MyList[]) {
  try { localStorage.setItem(KEY, JSON.stringify(items)); } catch { /* 容量超過・プライベートモード等は無視 */ }
}

export function addMyList(slug: string) {
  const items = loadMyLists();
  if (items.some((i) => i.slug === slug)) return;
  save([...items, { slug, createdAt: new Date().toISOString() }]);
}

export function setRemoved(slug: string, removed: boolean) {
  save(loadMyLists().map((i) => (i.slug === slug ? { ...i, removed } : i)));
}
