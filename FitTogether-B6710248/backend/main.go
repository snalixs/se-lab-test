package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

//go:embed sql/*.sql
var migrations embed.FS

type server struct {
	db         *pgxpool.Pool
	dev        bool
	mediaDir   string
	parser     *jwt.Parser
	secret     []byte
	reports    string
	httpClient *http.Client
}

type handler func(http.ResponseWriter, *http.Request, int64) error

type apiError struct {
	status int
	msg    string
}

func (e apiError) Error() string { return e.msg }
func bad(msg string) error       { return apiError{400, msg} }
func conflict(msg string) error  { return apiError{409, msg} }
func missing() error             { return apiError{404, "resource not found"} }
func forbidden() error           { return apiError{403, "access denied"} }

type dbReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func main() {
	migrate := flag.Bool("migrate", false, "apply schema migrations and exit")
	flag.Parse()
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("could not load .env: ", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL is required")
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = db.Ping(probe)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	devSetting := os.Getenv("APP_ENV") == "development"
	if *migrate {
		if err := applyMigrations(ctx, db, devSetting); err != nil {
			log.Fatal(err)
		}
		log.Print("FitTogether schema is up to date")
		return
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	s := &server{db: db, dev: devSetting, httpClient: &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	s.mediaDir, err = prepareMediaDir()
	if err != nil {
		log.Fatal(err)
	}
	if s.dev {
		host, _, err := net.SplitHostPort(addr)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !ip.IsLoopback() {
			log.Fatal("development authentication requires a loopback HTTP_ADDR")
		}
		log.Print("development authentication enabled")
	} else {
		s.secret = []byte(os.Getenv("AUTH_JWT_SECRET"))
		issuer, audience := os.Getenv("AUTH_JWT_ISSUER"), os.Getenv("AUTH_JWT_AUDIENCE")
		if len(s.secret) < 32 || issuer == "" || audience == "" {
			log.Fatal("configure AUTH_JWT_SECRET (32+ bytes), AUTH_JWT_ISSUER and AUTH_JWT_AUDIENCE")
		}
		s.parser = jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(issuer),
			jwt.WithAudience(audience), jwt.WithExpirationRequired(), jwt.WithLeeway(5*time.Second))
	}
	if endpoint := os.Getenv("REPORTS_API_URL"); endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Hostname() == "" || u.User != nil ||
			(u.Scheme != "https" && !(s.dev && u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) {
			log.Fatal("REPORTS_API_URL must be an HTTPS submit endpoint (loopback HTTP allowed only in dev)")
		}
		s.reports = u.String()
	}
	var schemaReady bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM fittogether_migrations WHERE name='004_post_archive.sql')").Scan(&schemaReady); err != nil || !schemaReady {
		log.Fatal("database schema is out of date; run go run . -migrate first")
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); s.runGroupWorker(workerCtx) }()
	defer func() { stopWorker(); <-workerDone }()
	srv := &http.Server{
		Addr: addr, Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 70 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("listening on %s", addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func applyMigrations(ctx context.Context, db *pgxpool.Pool, dev bool) error {
	return pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(52402117)"); err != nil {
			return err
		}
		files := []string{"001_features.sql", "002_group_lifecycle.sql", "003_comment_editing.sql", "004_post_archive.sql"}
		if dev {
			files = append([]string{"dev_members.sql"}, files...)
		}
		if _, err := tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS fittogether_migrations (name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"); err != nil {
			return err
		}
		for _, file := range files {
			tag, err := tx.Exec(ctx, "INSERT INTO fittogether_migrations(name) VALUES ($1) ON CONFLICT DO NOTHING", file)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				continue
			}
			stmt, err := migrations.ReadFile("sql/" + file)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, string(stmt), pgx.QueryExecModeSimpleProtocol); err != nil {
				return fmt.Errorf("migration %s: %w", file, err)
			}
		}
		return nil
	})
}

func (s *server) routes() http.Handler {
	m := http.NewServeMux()
	bind := func(pattern string, h handler) { m.HandleFunc(pattern, s.secure(h)) }
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if err := s.db.Ping(ctx); err != nil {
			writeJSON(w, 503, map[string]string{"status": "unavailable"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok", "image_upload": "multipart-10"})
	})
	m.HandleFunc("GET /media/{file}", s.serveMedia)
	bind("GET /api/posts", s.feed)
	bind("POST /api/posts", s.createPost)
	bind("GET /api/posts/{post}", s.getPost)
	bind("PATCH /api/posts/{post}", s.updatePost)
	bind("DELETE /api/posts/{post}", s.deletePost)
	bind("POST /api/posts/{post}/share", s.sharePost)
	bind("POST /api/posts/{post}/reports", s.reportPost)
	bind("GET /api/posts/{post}/comments", s.comments)
	bind("POST /api/posts/{post}/comments", s.createComment)
	bind("PATCH /api/posts/{post}/comments/{comment}", s.updateComment)
	bind("DELETE /api/posts/{post}/comments/{comment}", s.deleteComment)
	for _, method := range []string{"PUT", "DELETE"} {
		bind(method+" /api/posts/{post}/like", s.togglePost("post_likes"))
		bind(method+" /api/posts/{post}/bookmark", s.togglePost("post_bookmarks"))
		bind(method+" /api/users/{user}/follow", s.follow)
	}
	bind("GET /api/groups", s.groups)
	bind("POST /api/groups", s.createGroup)
	bind("GET /api/groups/{group}", s.getGroup)
	bind("PATCH /api/groups/{group}", s.updateGroup)
	bind("DELETE /api/groups/{group}", s.disband)
	bind("POST /api/groups/{group}/complete", s.completeGroup)
	bind("POST /api/groups/{group}/join", s.joinGroup)
	bind("DELETE /api/groups/{group}/membership", s.leaveGroup)
	bind("GET /api/groups/{group}/members", s.members)
	bind("DELETE /api/groups/{group}/members/{user}", s.removeMember)
	bind("PUT /api/groups/{group}/owner", s.transfer)
	bind("GET /api/groups/{group}/join-requests", s.joinRequests)
	bind("PUT /api/groups/{group}/join-requests/{request}", s.reviewRequest)
	bind("POST /api/groups/{group}/invites", s.invite)
	bind("GET /api/group-invites", s.myInvites)
	bind("PUT /api/groups/{group}/invites/{invite}", s.reviewInvite)
	bind("GET /api/groups/{group}/messages", s.messages)
	bind("POST /api/groups/{group}/messages", s.sendMessage)
	bind("GET /api/groups/{group}/events", retiredGroupEvents)
	bind("POST /api/groups/{group}/events", retiredGroupEvents)
	bind("PUT /api/groups/{group}/events/{event}/rsvp", retiredGroupEvents)
	bind("GET /api/group-notifications", s.notifications)
	bind("PUT /api/group-notifications/{notification}/read", s.markNotification)
	return m
}

func (s *server) secure(h handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		timeout := 8 * time.Second
		if r.Method == http.MethodPost && r.URL.Path == "/api/posts" &&
			strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			timeout = 60 * time.Second
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		r = r.WithContext(ctx)
		user, err := s.identity(r)
		if err == nil {
			err = h(w, r, user)
		}
		if err != nil {
			writeError(w, err)
		}
	}
}

func (s *server) identity(r *http.Request) (int64, error) {
	unauth := apiError{401, "valid bearer token required"}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 8192 {
		return 0, unauth
	}
	raw := parts[1]
	var sub string
	if s.dev {
		switch raw {
		case "dev-tom":
			sub = "1"
		case "dev-mint":
			sub = "2"
		case "dev-beam":
			sub = "3"
		default:
			return 0, unauth
		}
	} else {
		claims := &jwt.RegisteredClaims{}
		_, err := s.parser.ParseWithClaims(raw, claims, func(*jwt.Token) (any, error) { return s.secret, nil })
		if err != nil {
			return 0, unauth
		}
		sub = claims.Subject
	}
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil || id <= 0 {
		return 0, unauth
	}
	var verified, active bool
	err = s.db.QueryRow(r.Context(), `
		SELECT COALESCE(d->>'is_email_verified',d->>'isEmailVerified','false')='true',
			   COALESCE(d->>'account_status',d->>'accountStatus','')='active'
		  FROM (SELECT to_jsonb(u) d FROM users u WHERE u.id=$1) account`, id).Scan(&verified, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, unauth
	}
	if err != nil {
		return 0, err
	}
	if !verified || !active {
		return 0, forbidden()
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, err error) {
	var api apiError
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &api):
	case errors.Is(err, pgx.ErrNoRows):
		api = apiError{404, "resource not found"}
	case errors.As(err, &pg) && pg.Code == "23505":
		api = apiError{409, "record already exists"}
	case errors.As(err, &pg) && pg.Code == "23503":
		api = apiError{422, "referenced record does not exist or cannot be changed"}
	case errors.As(err, &pg) && pg.Code == "23514":
		api = apiError{422, "data violates a validation rule"}
	case errors.Is(err, context.DeadlineExceeded):
		api = apiError{504, "request timed out"}
	default:
		log.Printf("request failed: %v", err)
		api = apiError{500, "internal server error"}
	}
	if api.status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeJSON(w, api.status, map[string]string{"error": api.msg})
}

func decode(w http.ResponseWriter, r *http.Request, body any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(body); err != nil {
		return bad("invalid JSON body")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return bad("expected exactly one JSON object")
	}
	return nil
}

func idParam(r *http.Request, name string) (int64, error) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || v < 1 {
		return 0, bad("invalid " + name + " id")
	}
	return v, nil
}

func paging(r *http.Request) (int, int64, error) {
	page := r.URL.Query()
	limit, offset := 25, int64(0)
	if raw := page.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			return 0, 0, bad("limit must be between 1 and 100")
		}
		limit = n
	}
	if raw := page.Get("offset"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 || n > 100000 {
			return 0, 0, bad("offset must be between 0 and 100000")
		}
		offset = n
	}
	return limit, offset, nil
}

func jsonRow(w http.ResponseWriter, q dbReader, ctx context.Context, status int, sql string, args ...any) error {
	var value json.RawMessage
	if err := q.QueryRow(ctx, sql, args...).Scan(&value); err != nil {
		return err
	}
	writeJSON(w, status, value)
	return nil
}

func listQuery(w http.ResponseWriter, q dbReader, ctx context.Context, sql string, args ...any) error {
	return jsonRow(w, q, ctx, 200, "SELECT jsonb_build_object('items', COALESCE(jsonb_agg(to_jsonb(items)), '[]'::jsonb)) FROM ("+sql+") items", args...)
}

func clean(value string, min, max int) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	return length >= min && length <= max && !strings.ContainsRune(value, 0)
}
