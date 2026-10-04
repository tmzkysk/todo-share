export type Status = "todo" | "doing" | "done";
export interface Todo { id: number; title: string; status: Status }
export interface ListData { title: string; todos: Todo[] }

async function req<T>(url: string, method = "GET", body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) throw new Error(String(res.status));
  return res.status === 204 ? (undefined as T) : res.json();
}

export const api = {
  createList: (title: string) => req<{ slug: string }>("/api/lists", "POST", { title }),
  getList: (slug: string) => req<ListData>(`/api/lists/${slug}`),
  add: (slug: string, title: string) => req<Todo>(`/api/lists/${slug}/todos`, "POST", { title }),
  setStatus: (slug: string, id: number, status: Status) =>
    req<void>(`/api/lists/${slug}/todos/${id}`, "PATCH", { status }),
  remove: (slug: string, id: number) => req<void>(`/api/lists/${slug}/todos/${id}`, "DELETE"),
};
