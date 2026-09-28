package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type app struct {
	db         *sql.DB
	uploadDir  string
	maxFile    int64
	sessionTTL time.Duration
	dummyHash  []byte
}

type identity struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	ClassID   int64  `json:"class_id"`
	ClassName string `json:"class_name"`
}

type material struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Filename    string    `json:"filename"`
	ClassID     int64     `json:"class_id"`
	CreatedAt   time.Time `json:"created_at"`
	Content     string    `json:"content,omitempty"`
	StorageName string    `json:"-"`
}

func main() {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		log.Fatal("MYSQL_DSN is required")
	}
	for _, key := range []string{"TEACHER_A_PASSWORD", "STUDENT_A1_PASSWORD", "STUDENT_B1_PASSWORD"} {
		if os.Getenv(key) == "" {
			log.Fatalf("%s is required", key)
		}
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(12)
	db.SetConnMaxLifetime(3 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for {
		err = db.PingContext(ctx)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			log.Fatalf("database unavailable: %v", err)
		}
		time.Sleep(time.Second)
	}
	dir := envDefault("UPLOAD_DIR", "uploads")
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Fatal(err)
	}
	if err := initDatabase(ctx, db, dir); err != nil {
		log.Fatal(err)
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("not-the-right-password"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	a := &app{db: db, uploadDir: dir, maxFile: envInt("MAX_FILE_BYTES", 5<<20), sessionTTL: time.Duration(envInt("SESSION_HOURS", 8)) * time.Hour, dummyHash: dummy}
	addr := envDefault("ADDR", ":8081")
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, a.routes()))
}

func envDefault(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func envInt(key string, fallback int64) int64 {
	if n, err := strconv.ParseInt(os.Getenv(key), 10, 64); err == nil && n > 0 {
		return n
	}
	return fallback
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); io.WriteString(w, "ok\n") })
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", a.logout)
	mux.HandleFunc("GET /api/me", a.me)
	mux.HandleFunc("GET /api/materials", a.listMaterials)
	mux.HandleFunc("GET /api/search", a.searchKnowledge)
	mux.HandleFunc("POST /api/materials", a.upload)
	mux.HandleFunc("GET /api/materials/{id}", a.getMaterial)
	mux.HandleFunc("GET /api/materials/{id}/file", a.download)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not found") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func serverFailure(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	fail(w, http.StatusServiceUnavailable, "database unavailable")
}

func token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func tokenHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
func setSessionCookie(w http.ResponseWriter, value string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: value, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
}
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (a *app) currentUser(r *http.Request) (identity, int, error) {
	var u identity
	c, err := r.Cookie("session")
	if err != nil || len(c.Value) != 64 {
		return u, http.StatusUnauthorized, nil
	}
	err = a.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.role,u.class_id,c.name
		FROM sessions s JOIN users u ON u.id=s.user_id JOIN classes c ON c.id=u.class_id
		WHERE s.token_hash=? AND s.expires_at>UTC_TIMESTAMP()`, tokenHash(c.Value)).Scan(&u.ID, &u.Username, &u.Role, &u.ClassID, &u.ClassName)
	if errors.Is(err, sql.ErrNoRows) {
		return u, http.StatusUnauthorized, nil
	}
	if err != nil {
		return u, http.StatusServiceUnavailable, err
	}
	return u, 0, nil
}
func (a *app) requireUser(w http.ResponseWriter, r *http.Request) (identity, bool) {
	u, status, err := a.currentUser(r)
	if err != nil {
		serverFailure(w, err)
		return u, false
	}
	if status != 0 {
		fail(w, status, "login required")
		return u, false
	}
	return u, true
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		fail(w, 400, "JSON body required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil || dec.Decode(new(any)) != io.EOF || input.Username == "" || input.Password == "" {
		fail(w, 400, "invalid login request")
		return
	}
	var u identity
	var hash []byte
	err := a.db.QueryRowContext(r.Context(), `SELECT u.id,u.username,u.password_hash,u.role,u.class_id,c.name FROM users u JOIN classes c ON c.id=u.class_id WHERE u.username=?`, input.Username).Scan(&u.ID, &u.Username, &hash, &u.Role, &u.ClassID, &u.ClassName)
	if errors.Is(err, sql.ErrNoRows) {
		hash = a.dummyHash
	} else if err != nil {
		serverFailure(w, err)
		return
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(input.Password)) != nil || u.ID == 0 {
		fail(w, 401, "invalid username or password")
		return
	}
	value, err := token()
	if err != nil {
		fail(w, 500, "internal error")
		return
	}
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverFailure(w, err)
		return
	}
	defer tx.Rollback()
	if old, err := r.Cookie("session"); err == nil && len(old.Value) == 64 {
		if _, err := tx.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", tokenHash(old.Value)); err != nil {
			serverFailure(w, err)
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)", tokenHash(value), u.ID, time.Now().UTC().Add(a.sessionTTL)); err != nil {
		serverFailure(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverFailure(w, err)
		return
	}
	setSessionCookie(w, value, a.sessionTTL)
	writeJSON(w, 200, u)
}
func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil && len(c.Value) == 64 {
		if _, err := a.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE token_hash=?", tokenHash(c.Value)); err != nil {
			serverFailure(w, err)
			return
		}
	}
	clearSessionCookie(w)
	w.WriteHeader(204)
}
func (a *app) me(w http.ResponseWriter, r *http.Request) {
	if u, ok := a.requireUser(w, r); ok {
		writeJSON(w, 200, u)
	}
}

func (a *app) listMaterials(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	rows, err := a.db.QueryContext(r.Context(), "SELECT id,title,original_filename,class_id,created_at FROM materials WHERE class_id=? ORDER BY created_at DESC,id DESC", u.ClassID)
	if err != nil {
		serverFailure(w, err)
		return
	}
	defer rows.Close()
	items := make([]material, 0)
	for rows.Next() {
		var m material
		if err := rows.Scan(&m.ID, &m.Title, &m.Filename, &m.ClassID, &m.CreatedAt); err != nil {
			serverFailure(w, err)
			return
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		serverFailure(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func materialID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		fail(w, 404, "material not found")
		return 0, false
	}
	return id, true
}
func (a *app) findMaterial(r *http.Request, id int64, classID int64) (material, error) {
	var m material
	var actualClass int64
	err := a.db.QueryRowContext(r.Context(), `SELECT m.id,m.title,m.original_filename,m.storage_name,m.class_id,m.created_at,k.content
		FROM materials m JOIN knowledge_entries k ON k.material_id=m.id WHERE m.id=? AND m.class_id=?`, id, classID).Scan(&m.ID, &m.Title, &m.Filename, &m.StorageName, &actualClass, &m.CreatedAt, &m.Content)
	if err != nil {
		return m, err
	}
	m.ClassID = actualClass
	return m, nil
}
func (a *app) getMaterial(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id, ok := materialID(w, r)
	if !ok {
		return
	}
	m, err := a.findMaterial(r, id, u.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "material not found")
		return
	}
	if err != nil {
		serverFailure(w, err)
		return
	}
	writeJSON(w, 200, m)
}
func (a *app) download(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id, ok := materialID(w, r)
	if !ok {
		return
	}
	m, err := a.findMaterial(r, id, u.ClassID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "material not found")
		return
	}
	if err != nil {
		serverFailure(w, err)
		return
	}
	f, err := os.Open(filepath.Join(a.uploadDir, m.StorageName))
	if err != nil {
		log.Printf("download file error: %v", err)
		fail(w, 500, "file unavailable")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		fail(w, 500, "file unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": m.Filename}))
	http.ServeContent(w, r, m.Filename, info.ModTime(), f)
}

func cleanFilename(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = filepath.Base(value)
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
}
func (a *app) upload(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	if u.Role != "teacher" {
		fail(w, 403, "teacher required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.maxFile+(1<<20))
	parseErr := r.ParseMultipartForm(1 << 20)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err := parseErr; err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			fail(w, 413, "file too large")
		} else {
			fail(w, 400, "invalid multipart form")
		}
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		fail(w, 400, "file required")
		return
	}
	defer f.Close()
	filename := cleanFilename(header.Filename)
	ext := strings.ToLower(filepath.Ext(filename))
	if filename == "" || (ext != ".txt" && ext != ".md") {
		fail(w, 400, "only .txt and .md files are supported")
		return
	}
	if utf8.RuneCountInString(filename) > 255 {
		fail(w, 400, "filename too long")
		return
	}
	title := filename[:len(filename)-len(filepath.Ext(filename))]
	if title == "" {
		fail(w, 400, "filename needs a title")
		return
	}
	content, err := io.ReadAll(io.LimitReader(f, a.maxFile+1))
	if err != nil {
		fail(w, 400, "cannot read file")
		return
	}
	if int64(len(content)) > a.maxFile {
		fail(w, 413, "file too large")
		return
	}
	if len(content) == 0 || !utf8.Valid(content) {
		fail(w, 400, "file must contain non-empty UTF-8 text")
		return
	}
	storage, err := token()
	if err != nil {
		fail(w, 500, "internal error")
		return
	}
	storage += ext
	path := filepath.Join(a.uploadDir, storage)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		log.Printf("upload file error: %v", err)
		fail(w, 500, "cannot save file")
		return
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		os.Remove(path)
		fail(w, 500, "cannot save file")
		return
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		fail(w, 500, "cannot save file")
		return
	}
	committed := false
	defer func() {
		if !committed {
			if err := os.Remove(path); err != nil {
				log.Printf("upload cleanup error: %v", err)
			}
		}
	}()
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverFailure(w, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(), "INSERT INTO materials(title,original_filename,storage_name,uploaded_by,class_id) VALUES(?,?,?,?,?)", title, filename, storage, u.ID, u.ClassID)
	if err != nil {
		serverFailure(w, err)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		serverFailure(w, err)
		return
	}
	if _, err := tx.ExecContext(r.Context(), "INSERT INTO knowledge_entries(material_id,class_id,content) VALUES(?,?,?)", id, u.ClassID, string(content)); err != nil {
		serverFailure(w, err)
		return
	}
	if err := insertKnowledgeChunks(r.Context(), tx, id, u.ClassID, string(content)); err != nil {
		serverFailure(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverFailure(w, err)
		return
	}
	committed = true
	writeJSON(w, 201, material{ID: id, Title: title, Filename: filename, ClassID: u.ClassID, CreatedAt: time.Now().UTC()})
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS classes (id BIGINT PRIMARY KEY, name VARCHAR(100) NOT NULL UNIQUE) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS users (id BIGINT AUTO_INCREMENT PRIMARY KEY, username VARCHAR(100) NOT NULL UNIQUE, password_hash VARCHAR(100) NOT NULL, role ENUM('teacher','student') NOT NULL, class_id BIGINT NOT NULL, FOREIGN KEY(class_id) REFERENCES classes(id), INDEX(class_id)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS sessions (token_hash CHAR(64) PRIMARY KEY, user_id BIGINT NOT NULL, expires_at DATETIME(6) NOT NULL, FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE, INDEX(expires_at)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS materials (id BIGINT AUTO_INCREMENT PRIMARY KEY, title VARCHAR(255) NOT NULL, original_filename VARCHAR(255) NOT NULL, storage_name VARCHAR(100) NOT NULL UNIQUE, uploaded_by BIGINT NOT NULL, class_id BIGINT NOT NULL, created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6), FOREIGN KEY(uploaded_by) REFERENCES users(id), FOREIGN KEY(class_id) REFERENCES classes(id), INDEX(class_id)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS knowledge_entries (material_id BIGINT PRIMARY KEY, class_id BIGINT NOT NULL, content MEDIUMTEXT NOT NULL, FOREIGN KEY(material_id) REFERENCES materials(id) ON DELETE CASCADE, FOREIGN KEY(class_id) REFERENCES classes(id), INDEX(class_id)) ENGINE=InnoDB`,
	`CREATE TABLE IF NOT EXISTS knowledge_chunks (id BIGINT AUTO_INCREMENT PRIMARY KEY, material_id BIGINT NOT NULL, chunk_index INT NOT NULL, class_id BIGINT NOT NULL, char_start INT NOT NULL, char_end INT NOT NULL, content TEXT NOT NULL, UNIQUE KEY(material_id,chunk_index), FOREIGN KEY(material_id) REFERENCES materials(id) ON DELETE CASCADE, FOREIGN KEY(class_id) REFERENCES classes(id), INDEX(class_id), FULLTEXT KEY ft_content(content) WITH PARSER ngram) ENGINE=InnoDB`,
}

func initDatabase(ctx context.Context, db *sql.DB, dir string) error {
	for _, stmt := range schema {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	for id, name := range map[int]string{1: "A班", 2: "B班"} {
		if _, err := db.ExecContext(ctx, "INSERT IGNORE INTO classes(id,name) VALUES(?,?)", id, name); err != nil {
			return err
		}
	}
	for _, seed := range []struct {
		username, env, role string
		classID             int
	}{
		{"teacher_a", "TEACHER_A_PASSWORD", "teacher", 1}, {"student_a1", "STUDENT_A1_PASSWORD", "student", 1}, {"student_b1", "STUDENT_B1_PASSWORD", "student", 2},
	} {
		hash, err := bcrypt.GenerateFromPassword([]byte(os.Getenv(seed.env)), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, "INSERT IGNORE INTO users(username,password_hash,role,class_id) VALUES(?,?,?,?)", seed.username, string(hash), seed.role, seed.classID); err != nil {
			return err
		}
	}
	for _, seed := range []struct {
		classID       int
		name, content string
	}{
		{1, "A班示例.txt", "A班示例材料\n这份材料仅供A班查看。\n"},
		{2, "B班示例.txt", "B班示例材料\n这份材料仅供B班查看。\n"},
	} {
		storage := fmt.Sprintf("seed-class-%d.txt", seed.classID)
		var existing int
		err := db.QueryRowContext(ctx, "SELECT 1 FROM materials WHERE storage_name=?", storage).Scan(&existing)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		path := filepath.Join(dir, storage)
		if err := os.WriteFile(path, []byte(seed.content), 0600); err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		var userID int64
		err = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE username='teacher_a'").Scan(&userID)
		if err == nil {
			var result sql.Result
			result, err = tx.ExecContext(ctx, "INSERT INTO materials(title,original_filename,storage_name,uploaded_by,class_id) VALUES(?,?,?,?,?)", strings.TrimSuffix(seed.name, ".txt"), seed.name, storage, userID, seed.classID)
			if err == nil {
				var id int64
				id, err = result.LastInsertId()
				if err == nil {
					_, err = tx.ExecContext(ctx, "INSERT INTO knowledge_entries(material_id,class_id,content) VALUES(?,?,?)", id, seed.classID, seed.content)
				}
			}
		}
		if err != nil {
			tx.Rollback()
			os.Remove(path)
			return err
		}
		if err := tx.Commit(); err != nil {
			os.Remove(path)
			return err
		}
	}
	return backfillKnowledgeChunks(ctx, db)
}
