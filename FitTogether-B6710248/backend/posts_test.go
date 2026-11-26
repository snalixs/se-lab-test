package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("post_test_%d", time.Now().UnixNano())
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+ident); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, e := admin.Exec(ctx, "DROP SCHEMA "+ident+" CASCADE"); e != nil {
			t.Error(e)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = applyMigrations(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	if err = applyMigrations(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	s := &server{db: db}
	call := func(h handler, post, comment, user int64, method, body string) (*httptest.ResponseRecorder, error) {
		r := httptest.NewRequest(method, "/", strings.NewReader(body))
		r.SetPathValue("post", fmt.Sprint(post))
		r.SetPathValue("comment", fmt.Sprint(comment))
		w := httptest.NewRecorder()
		return w, h(w, r, user)
	}
	create := func(visibility string) int64 {
		t.Helper()
		w, e := call(s.createPost, 0, 0, 1, "POST", `{"content":"Original","visibility":"`+visibility+`","images":["https://example.com/image.png"],"hashtags":["running"]}`)
		if e != nil {
			t.Fatal(e)
		}
		var p struct {
			ID int64 `json:"id"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &p); e != nil {
			t.Fatal(e)
		}
		return p.ID
	}
	addComment := func(post, parent, user int64) int64 {
		t.Helper()
		body := `{"content":"Comment"}`
		if parent != 0 {
			body = fmt.Sprintf(`{"content":"Reply","parent_comment_id":%d}`, parent)
		}
		w, e := call(s.createComment, post, 0, user, "POST", body)
		if e != nil {
			t.Fatal(e)
		}
		var c struct {
			ID int64 `json:"id"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &c); e != nil {
			t.Fatal(e)
		}
		return c.ID
	}
	t.Run("audience_and_comment_ownership", func(t *testing.T) {
		post := create("followers")
		if _, e := call(s.getPost, post, 0, 2, "GET", ""); e == nil {
			t.Fatal("non-follower read restricted post")
		}
		if _, e := db.Exec(ctx, "INSERT INTO user_follows(follower_id,followed_id) VALUES (2,1)"); e != nil {
			t.Fatal(e)
		}
		if _, e := call(s.getPost, post, 0, 2, "GET", ""); e != nil {
			t.Fatal(e)
		}
		if _, e := call(s.sharePost, post, 0, 2, "POST", `{"content":"share"}`); e == nil {
			t.Fatal("shared followers-only post")
		}
		comment := addComment(post, 0, 2)
		if _, e := call(s.updateComment, post, comment, 1, "PATCH", `{"content":"hijack"}`); e == nil {
			t.Fatal("post owner edited another author's comment")
		}
		if _, e := call(s.deleteComment, post, comment, 1, "DELETE", ""); e == nil {
			t.Fatal("post owner deleted another author's comment")
		}
		if _, e := call(s.updateComment, post, comment, 2, "PATCH", `{"content":"  edited  "}`); e != nil {
			t.Fatal(e)
		}
		var ok bool
		if e := db.QueryRow(ctx, "SELECT content='edited' AND updated_at IS NOT NULL FROM comments WHERE id=$1", comment).Scan(&ok); e != nil || !ok {
			t.Fatal("edit not persisted", e)
		}
		if _, e := call(s.updateComment, post, comment, 2, "PATCH", `{"content":" "}`); e == nil {
			t.Fatal("blank edit accepted")
		}
		reply := addComment(post, comment, 1)
		if _, e := call(s.deleteComment, post, comment, 2, "DELETE", ""); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(ctx, "SELECT parent_comment_id IS NULL FROM comments WHERE id=$1", reply).Scan(&ok); e != nil || !ok {
			t.Fatal("reply lost", e)
		}
	})
	t.Run("archive_and_preserve_shares", func(t *testing.T) {
		post := create("public")
		parent := addComment(post, 0, 2)
		addComment(post, parent, 3)
		for _, table := range []string{"post_likes", "post_bookmarks"} {
			if _, e := db.Exec(ctx, "INSERT INTO "+table+"(post_id,user_id) VALUES ($1,2)", post); e != nil {
				t.Fatal(e)
			}
		}
		w, e := call(s.sharePost, post, 0, 2, "POST", `{"content":""}`)
		if e != nil {
			t.Fatal(e)
		}
		var shared struct {
			ID int64 `json:"id"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &shared); e != nil {
			t.Fatal(e)
		}
		shareComment := addComment(shared.ID, 0, 3)
		if _, e = call(s.deletePost, post, 0, 2, "DELETE", ""); e == nil {
			t.Fatal("another user deleted post")
		}
		if _, e = call(s.deletePost, post, 0, 1, "DELETE", ""); e != nil {
			t.Fatal(e)
		}
		if _, e = call(s.deletePost, post, 0, 1, "DELETE", ""); e != nil {
			t.Fatal("repeat delete", e)
		}
		var ok bool
		e = db.QueryRow(ctx, `SELECT post_data->>'content'='Original'
   AND jsonb_array_length(related_data->'images')=1
   AND jsonb_array_length(related_data->'comments')=2
   AND jsonb_array_length(related_data->'likes')=1
   AND jsonb_array_length(related_data->'bookmarks')=1
   AND jsonb_array_length(related_data->'hashtags')=1
   AND NOT EXISTS(SELECT 1 FROM posts WHERE id=$1)
   AND NOT EXISTS(SELECT 1 FROM comments WHERE post_id=$1)
   FROM deleted_posts WHERE original_post_id=$1`, post).Scan(&ok)
		if e != nil || !ok {
			t.Fatal("archive incomplete or source retained", e)
		}
		w, e = call(s.getPost, shared.ID, 0, 3, "GET", "")
		if e != nil {
			t.Fatal("share disappeared", e)
		}
		var result struct {
			Deleted bool   `json:"shared_post_deleted"`
			Content string `json:"content"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil || !result.Deleted || result.Content != "" {
			t.Fatal("invalid share placeholder", e, w.Body.String())
		}
		if e = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM comments WHERE id=$1)", shareComment).Scan(&ok); e != nil || !ok {
			t.Fatal("share interactions lost", e)
		}
		if _, e = call(s.sharePost, shared.ID, 0, 3, "POST", `{"content":"again"}`); e == nil {
			t.Fatal("re-shared a missing original")
		}
		if _, e = call(s.updatePost, shared.ID, 0, 2, "PATCH", `{"content":""}`); e != nil {
			t.Fatal("empty share caption cannot be edited", e)
		}
	})
	t.Run("failed_archive_rolls_back_deletion", func(t *testing.T) {
		post := create("public")
		if _, e := db.Exec(ctx, `INSERT INTO deleted_posts(original_post_id,author_id,deleted_by,post_data,related_data) VALUES ($1,1,1,'{}','{}')`, post); e != nil {
			t.Fatal(e)
		}
		if _, e := call(s.deletePost, post, 0, 1, "DELETE", ""); e == nil {
			t.Fatal("expected archive conflict")
		}
		var exists bool
		if e := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM posts WHERE id=$1)", post).Scan(&exists); e != nil || !exists {
			t.Fatal("source deleted despite failed archive", e)
		}
	})
}
