package main

import (
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
)

//go:embed schema.sql
var schema string

type Todo struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type server struct{ db *sql.DB }

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func validTitle(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	return s, n > 0 && n <= max
}

func decode(w http.ResponseWriter, r *http.Request, v any) {
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v)
}

func (s *server) createList(w http.ResponseWriter, r *http.Request) {
	var in struct{ Title string `json:"title"` }
	decode(w, r, &in)
	title, ok := validTitle(in.Title, 100)
	if !ok {
		title = "TODOリスト"
	}
	b := make([]byte, 10)
	rand.Read(b)
	slug := hex.EncodeToString(b) // 推測困難な20文字。URLを知る人だけが編集可能
	if _, err := s.db.Exec("INSERT INTO lists (slug, title) VALUES (?, ?)", slug, title); err != nil {
		fail(w, 500, "db error")
		return
	}
	writeJSON(w, 201, map[string]string{"slug": slug})
}

func (s *server) getList(w http.ResponseWriter, r *http.Request) {
	var id int64
	var title string
	if err := s.db.QueryRow("SELECT id, title FROM lists WHERE slug=?", r.PathValue("slug")).Scan(&id, &title); err != nil {
		fail(w, 404, "not found")
		return
	}
	rows, err := s.db.Query("SELECT id, title, status FROM todos WHERE list_id=? ORDER BY id", id)
	if err != nil {
		fail(w, 500, "db error")
		return
	}
	defer rows.Close()
	todos := []Todo{}
	for rows.Next() {
		var t Todo
		rows.Scan(&t.ID, &t.Title, &t.Status)
		todos = append(todos, t)
	}
	writeJSON(w, 200, map[string]any{"title": title, "todos": todos})
}

func (s *server) addTodo(w http.ResponseWriter, r *http.Request) {
	var in struct{ Title string `json:"title"` }
	decode(w, r, &in)
	title, ok := validTitle(in.Title, 200)
	if !ok {
		fail(w, 400, "title must be 1-200 chars")
		return
	}
	res, err := s.db.Exec("INSERT INTO todos (list_id, title) SELECT id, ? FROM lists WHERE slug=?", title, r.PathValue("slug"))
	if err != nil {
		fail(w, 500, "db error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, 404, "not found")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, 201, Todo{ID: id, Title: title, Status: "todo"})
}

func (s *server) updateTodo(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	slug := r.PathValue("slug")
	var in struct {
		Status *string `json:"status"`
		Title  *string `json:"title"`
	}
	decode(w, r, &in)
	if in.Status != nil {
		switch *in.Status {
		case "todo", "doing", "done":
			s.db.Exec("UPDATE todos t JOIN lists l ON l.id=t.list_id SET t.status=? WHERE l.slug=? AND t.id=?", *in.Status, slug, id)
		default:
			fail(w, 400, "invalid status")
			return
		}
	}
	if in.Title != nil {
		t, ok := validTitle(*in.Title, 200)
		if !ok {
			fail(w, 400, "title must be 1-200 chars")
			return
		}
		s.db.Exec("UPDATE todos t JOIN lists l ON l.id=t.list_id SET t.title=? WHERE l.slug=? AND t.id=?", t, slug, id)
	}
	w.WriteHeader(204)
}

func (s *server) deleteTodo(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.db.Exec("DELETE t FROM todos t JOIN lists l ON l.id=t.list_id WHERE l.slug=? AND t.id=?", r.PathValue("slug"), id)
	w.WriteHeader(204)
}

// 静的ファイル配信 + SPAフォールバック
func spa(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			http.ServeFile(w, r, filepath.Join(dir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	})
}

func main() {
	db, err := sql.Open("mysql", env("DB_DSN", "root:root@tcp(127.0.0.1:3306)/todo?parseTime=true&charset=utf8mb4"))
	if err != nil {
		log.Fatal(err)
	}
	for i := 0; i < 30; i++ { // DB起動待ち
		if err = db.Ping(); err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	for _, stmt := range strings.Split(schema, ";") {
		if strings.TrimSpace(stmt) != "" {
			if _, err := db.Exec(stmt); err != nil {
				log.Fatal(err)
			}
		}
	}
	s := &server{db}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/lists", s.createList)
	mux.HandleFunc("GET /api/lists/{slug}", s.getList)
	mux.HandleFunc("POST /api/lists/{slug}/todos", s.addTodo)
	mux.HandleFunc("PATCH /api/lists/{slug}/todos/{id}", s.updateTodo)
	mux.HandleFunc("DELETE /api/lists/{slug}/todos/{id}", s.deleteTodo)
	mux.Handle("/", spa(env("STATIC_DIR", "../frontend/dist")))
	addr := ":" + env("PORT", "8080")
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
