package main

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

//go:embed schema.sql
var schema string

type Todo struct {
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type server struct{ db *sql.DB }

var ready atomic.Bool // DB接続・テーブル作成が終わったらtrue

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

// Aiven等、独自CAで署名されたMySQLに接続するための設定。
// 環境変数 DB_CA_PEM にCA証明書(ca.pem)の中身を入れ、DSNに tls=aiven を付ける。
// 証明書チェーンはCAで検証する（MySQLのVERIFY_CA相当。ホスト名検証は行わない）。
func registerTLS() {
	pem := strings.ReplaceAll(os.Getenv("DB_CA_PEM"), `\n`, "\n")
	if strings.TrimSpace(pem) == "" {
		return
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(pem)) {
		log.Print("DB_CA_PEM の中身がPEM形式として読めません。ca.pem の -----BEGIN から END----- までを貼り付けてください")
		return
	}
	err := mysql.RegisterTLSConfig("aiven", &tls.Config{
		InsecureSkipVerify: true, // 標準のホスト名検証は無効化し、下で自前検証する
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if len(raw) == 0 {
				return x509.UnknownAuthorityError{}
			}
			certs := make([]*x509.Certificate, 0, len(raw))
			for _, r := range raw {
				c, err := x509.ParseCertificate(r)
				if err != nil {
					return err
				}
				certs = append(certs, c)
			}
			opts := x509.VerifyOptions{Roots: pool, Intermediates: x509.NewCertPool()}
			for _, c := range certs[1:] {
				opts.Intermediates.AddCert(c)
			}
			_, err := certs[0].Verify(opts)
			return err
		},
	})
	if err != nil {
		log.Printf("TLS設定の登録に失敗: %v", err)
	}
}

// Aivenの「Service URI」(mysql://user:pass@host:port/db?ssl-mode=REQUIRED) もそのまま使えるよう、
// Goドライバ形式のDSNに変換する。それ以外の形式はそのまま返す。
func buildDSN(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "mysql://") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	pw, _ := u.User.Password()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = u.User.Username(), pw
	cfg.Net, cfg.Addr = "tcp", u.Host
	cfg.DBName = strings.TrimPrefix(u.Path, "/")
	cfg.ParseTime = true
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	if u.Query().Get("ssl-mode") != "" && strings.TrimSpace(os.Getenv("DB_CA_PEM")) != "" {
		cfg.TLSConfig = "aiven"
	}
	return cfg.FormatDSN()
}

// DBへの接続とテーブル作成。失敗してもプロセスは落とさず、原因をログに出して再試行する
// （ポートを先に開いておくことで、Renderの「ポート未検出」で原因が隠れるのを防ぐ）
func (s *server) connect() {
	dsn := buildDSN(env("DB_DSN", "root:root@tcp(127.0.0.1:3306)/todo?parseTime=true&charset=utf8mb4"))
	// パスワードを除いた接続先を表示（設定ミスの切り分け用）
	if cfg, err := mysql.ParseDSN(dsn); err != nil {
		log.Printf("DB_DSN を解釈できません: %v", err)
	} else {
		log.Printf("DB接続先: user=%q addr=%q db=%q tls=%q", cfg.User, cfg.Addr, cfg.DBName, cfg.TLSConfig)
	}
	for {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			log.Printf("DB_DSN が不正です（tls=aiven を使う場合は DB_CA_PEM も必須）: %v", err)
			time.Sleep(10 * time.Second)
			continue
		}
		db.SetMaxOpenConns(5)
		db.SetConnMaxLifetime(3 * time.Minute)
		if err := db.Ping(); err != nil {
			log.Printf("DB接続に失敗（ホスト/ポート/パスワード/TLS設定を確認）: %v", err)
			db.Close()
			time.Sleep(5 * time.Second)
			continue
		}
		ok := true
		for _, stmt := range strings.Split(schema, ";") {
			if strings.TrimSpace(stmt) == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				log.Printf("テーブル作成に失敗: %v", err)
				ok = false
				break
			}
		}
		if !ok {
			db.Close()
			time.Sleep(5 * time.Second)
			continue
		}
		s.db = db
		ready.Store(true)
		log.Println("DB ready")
		return
	}
}

func main() {
	registerTLS()
	s := &server{}
	go s.connect()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/lists", s.createList)
	mux.HandleFunc("GET /api/lists/{slug}", s.getList)
	mux.HandleFunc("POST /api/lists/{slug}/todos", s.addTodo)
	mux.HandleFunc("PATCH /api/lists/{slug}/todos/{id}", s.updateTodo)
	mux.HandleFunc("DELETE /api/lists/{slug}/todos/{id}", s.deleteTodo)
	mux.Handle("/", spa(env("STATIC_DIR", "../frontend/dist")))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && !ready.Load() {
			fail(w, 503, "database not ready")
			return
		}
		mux.ServeHTTP(w, r)
	})
	addr := ":" + env("PORT", "8080")
	log.Println("listening on", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}
