package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

// The same visibility checks cover all user-facing reads and interactions.
const postVisible = `p.status='published' AND (
	p.author_id=$1 OR p.visibility='public' OR EXISTS (
		SELECT 1 FROM user_follows f WHERE f.follower_id=$1 AND f.followed_id=p.author_id
	)
) AND (p.shared_post_id IS NULL OR EXISTS (
	SELECT 1 FROM posts origin WHERE origin.id=p.shared_post_id AND origin.status='published'
	  AND (origin.author_id=$1 OR origin.visibility='public' OR EXISTS (
		SELECT 1 FROM user_follows f WHERE f.follower_id=$1 AND f.followed_id=origin.author_id
	  ))
))`

const postSelect = `SELECT p.id,p.author_id,p.shared_post_id,p.content,p.visibility,p.status,
	(p.deleted_shared_post_id IS NOT NULL) AS shared_post_deleted,
	p.created_at,p.updated_at,profile.display_name,profile.photo_url,
	COALESCE((SELECT jsonb_agg(i.image_url ORDER BY i.sort_order) FROM post_images i WHERE i.post_id=p.id),'[]') AS images,
	COALESCE((SELECT jsonb_agg(h.name ORDER BY h.name) FROM post_hashtags ph JOIN hashtags h ON h.id=ph.hashtag_id WHERE ph.post_id=p.id),'[]') AS hashtags,
	(SELECT count(*) FROM comments c WHERE c.post_id=p.id) AS comment_count,
	(SELECT count(*) FROM post_likes l WHERE l.post_id=p.id) AS like_count,
	EXISTS (SELECT 1 FROM post_likes l WHERE l.post_id=p.id AND l.user_id=$1) AS liked,
	EXISTS (SELECT 1 FROM post_bookmarks b WHERE b.post_id=p.id AND b.user_id=$1) AS bookmarked
	FROM posts p LEFT JOIN profiles profile ON profile.user_id=p.author_id WHERE ` + postVisible

type postInput struct {
	Content    string   `json:"content"`
	Visibility string   `json:"visibility"`
	Images     []string `json:"images"`
	Hashtags   []string `json:"hashtags"`
}

func validatePost(body *postInput, min int) error {
	body.Content = strings.TrimSpace(body.Content)
	if !clean(body.Content, min, 1500) {
		return bad("invalid post content")
	}
	if body.Visibility != "public" && body.Visibility != "followers" {
		return bad("visibility must be public or followers")
	}
	if len(body.Images) > 10 || len(body.Hashtags) > 10 {
		return bad("at most 10 images and 10 hashtags are allowed")
	}
	for _, image := range body.Images {
		if !validMediaURL(image) {
			return bad("image URLs must be HTTPS or uploaded media paths")
		}
	}
	for i, h := range body.Hashtags {
		h = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(h), "#"))
		if !clean(h, 1, 60) || strings.ContainsFunc(h, func(r rune) bool {
			return !(unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) || r == '_')
		}) {
			return bad("invalid hashtag")
		}
		body.Hashtags[i] = h
	}
	return nil
}

func validMediaURL(value string) bool {
	if isLocalMediaURL(value) {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && len(value) <= 2048 && u.Scheme == "https" && u.Hostname() != "" && u.User == nil
}

func replacePostDetails(ctx context.Context, tx pgx.Tx, post int64, body postInput) error {
	if body.Images != nil {
		if _, err := tx.Exec(ctx, "DELETE FROM post_images WHERE post_id=$1", post); err != nil {
			return err
		}
		for i, image := range body.Images {
			if _, err := tx.Exec(ctx, "INSERT INTO post_images(post_id,image_url,sort_order) VALUES ($1,$2,$3)", post, image, i); err != nil {
				return err
			}
		}
	}
	if body.Hashtags != nil {
		if _, err := tx.Exec(ctx, "DELETE FROM post_hashtags WHERE post_id=$1", post); err != nil {
			return err
		}
		slices.Sort(body.Hashtags)
		for _, h := range slices.Compact(body.Hashtags) {
			if _, err := tx.Exec(ctx, "INSERT INTO hashtags(name) VALUES ($1) ON CONFLICT DO NOTHING", h); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO post_hashtags(post_id,hashtag_id) SELECT $1,id FROM hashtags WHERE name=$2 ON CONFLICT DO NOTHING", post, h); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *server) feed(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	query := r.URL.Query()
	mode, q := query.Get("mode"), strings.TrimSpace(query.Get("q"))
	if mode == "" {
		mode = "latest"
	}
	switch mode {
	case "latest", "following", "popular", "bookmarked":
	default:
		return bad("unknown feed mode")
	}
	if !clean(q, 0, 100) {
		return bad("search query is too long")
	}
	return listQuery(w, s.db, r.Context(), postSelect+`
		AND ($2::text <> 'following' OR p.author_id=$1 OR EXISTS (
			SELECT 1 FROM user_follows f WHERE f.follower_id=$1 AND f.followed_id=p.author_id))
		AND ($2::text <> 'bookmarked' OR EXISTS (
			SELECT 1 FROM post_bookmarks b WHERE b.post_id=p.id AND b.user_id=$1))
		AND ($3::text = '' OR (
			(left($3,1)<>'#' AND strpos(lower(p.content),lower($3))>0) OR EXISTS (
			SELECT 1 FROM post_hashtags ph JOIN hashtags h ON h.id=ph.hashtag_id
			WHERE ph.post_id=p.id AND h.name=lower(ltrim($3,'#')))))
		ORDER BY CASE WHEN $2='popular' THEN (SELECT count(*) FROM post_likes pl WHERE pl.post_id=p.id) ELSE 0 END DESC,
			p.id DESC LIMIT $4 OFFSET $5`, user, mode, q, limit, offset)
}

func (s *server) getPost(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "post")
	if err != nil {
		return err
	}
	return s.postResponse(w, r.Context(), user, id, 200)
}

func (s *server) postResponse(w http.ResponseWriter, ctx context.Context, user, id int64, status int) error {
	return jsonRow(w, s.db, ctx, status, "SELECT to_jsonb(record) FROM ("+postSelect+" AND p.id=$2) record", user, id)
}

func (s *server) createPost(w http.ResponseWriter, r *http.Request, user int64) error {
	var body postInput
	var mediaPaths []string
	var err error
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		body, mediaPaths, err = s.readMultipartPost(w, r)
	} else {
		err = decode(w, r, &body)
	}
	if err != nil {
		return err
	}
	defer func() {
		if mediaPaths != nil {
			removeMedia(mediaPaths)
		}
	}()
	if body.Visibility == "" {
		body.Visibility = "public"
	}
	if err := validatePost(&body, 1); err != nil {
		return err
	}
	ctx := r.Context()
	var id int64
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO posts(author_id,content,visibility) VALUES ($1,$2,$3)
			RETURNING id`, user, body.Content, body.Visibility).Scan(&id); err != nil {
			return err
		}
		return replacePostDetails(ctx, tx, id, body)
	})
	if err != nil {
		return err
	}
	mediaPaths = nil
	return s.postResponse(w, ctx, user, id, 201)
}

func (s *server) updatePost(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "post")
	if err != nil {
		return err
	}
	var patch struct {
		Content    *string  `json:"content"`
		Visibility *string  `json:"visibility"`
		Images     []string `json:"images"`
		Hashtags   []string `json:"hashtags"`
	}
	if err := decode(w, r, &patch); err != nil {
		return err
	}
	ctx := r.Context()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		body := postInput{Images: patch.Images, Hashtags: patch.Hashtags}
		var shared bool
		err := tx.QueryRow(ctx, "SELECT content,visibility,(shared_post_id IS NOT NULL OR deleted_shared_post_id IS NOT NULL) FROM posts WHERE id=$1 AND author_id=$2 AND status='published' FOR UPDATE",
			id, user).Scan(&body.Content, &body.Visibility, &shared)
		if err != nil {
			return err
		}
		if patch.Content != nil {
			body.Content = *patch.Content
		}
		if patch.Visibility != nil {
			body.Visibility = *patch.Visibility
		}
		min := 1
		if shared {
			min = 0
		}
		if err := validatePost(&body, min); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE posts SET content=$2,visibility=$3,updated_at=now() WHERE id=$1", id, body.Content, body.Visibility); err != nil {
			return err
		}
		return replacePostDetails(ctx, tx, id, body)
	})
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}

func (s *server) sharePost(w http.ResponseWriter, r *http.Request, user int64) error {
	target, err := idParam(r, "post")
	if err != nil {
		return err
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	if !clean(body.Content, 0, 1500) {
		return bad("invalid share caption")
	}
	ctx := r.Context()
	var id int64
	err = s.db.QueryRow(ctx, `INSERT INTO posts(author_id,shared_post_id,content,visibility)
		SELECT $1,origin.id,$3,'public' FROM posts p JOIN posts origin ON origin.id=COALESCE(p.shared_post_id,p.id)
		WHERE p.id=$2 AND p.deleted_shared_post_id IS NULL AND p.visibility='public' AND origin.visibility='public' AND `+postVisible+` RETURNING id`,
		user, target, strings.TrimSpace(body.Content)).Scan(&id)
	if err != nil {
		return err
	}
	return s.postResponse(w, ctx, user, id, 201)
}

func checkPost(ctx context.Context, q dbReader, user, id int64) error {
	var found int64
	return q.QueryRow(ctx, "SELECT p.id FROM posts p WHERE p.id=$2 AND "+postVisible+" FOR UPDATE OF p", user, id).Scan(&found)
}

func (s *server) togglePost(table string) handler {
	return func(w http.ResponseWriter, r *http.Request, user int64) error {
		id, err := idParam(r, "post")
		if err != nil {
			return err
		}
		ctx := r.Context()
		err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
			if err := checkPost(ctx, tx, user, id); err != nil {
				return err
			}
			if r.Method == http.MethodPut {
				_, err := tx.Exec(ctx, "INSERT INTO "+table+" (post_id,user_id) VALUES ($1,$2) ON CONFLICT DO NOTHING", id, user)
				return err
			}
			_, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE post_id=$1 AND user_id=$2", id, user)
			return err
		})
		if err != nil {
			return err
		}
		w.WriteHeader(204)
		return nil
	}
}

func (s *server) follow(w http.ResponseWriter, r *http.Request, user int64) error {
	target, err := idParam(r, "user")
	if err != nil {
		return err
	}
	if target == user {
		return bad("cannot follow yourself")
	}
	if r.Method == http.MethodPut {
		_, err = s.db.Exec(r.Context(), "INSERT INTO user_follows(follower_id,followed_id) VALUES ($1,$2) ON CONFLICT DO NOTHING", user, target)
	} else {
		_, err = s.db.Exec(r.Context(), "DELETE FROM user_follows WHERE follower_id=$1 AND followed_id=$2", user, target)
	}
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}

func (s *server) comments(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "post")
	if err != nil {
		return err
	}
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	if err := checkPost(ctx, s.db, user, id); err != nil {
		return err
	}
	return listQuery(w, s.db, ctx, `SELECT c.*, profile.display_name,profile.photo_url FROM comments c
		JOIN posts p ON p.id=c.post_id LEFT JOIN profiles profile ON profile.user_id=c.author_id
		WHERE c.post_id=$2 AND `+postVisible+` ORDER BY c.id LIMIT $3 OFFSET $4`, user, id, limit, offset)
}

func (s *server) createComment(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "post")
	if err != nil {
		return err
	}
	var body struct {
		Content  string `json:"content"`
		ParentID *int64 `json:"parent_comment_id"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	if !clean(body.Content, 1, 350) || (body.ParentID != nil && *body.ParentID < 1) {
		return bad("invalid comment")
	}
	var data json.RawMessage
	ctx := r.Context()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := checkPost(ctx, tx, user, id); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `WITH inserted AS (
		INSERT INTO comments(post_id,author_id,parent_comment_id,content)
		SELECT p.id,$1,$3,$4 FROM posts p WHERE p.id=$2 AND `+postVisible+`
		RETURNING *) SELECT to_jsonb(inserted) FROM inserted`, user, id, body.ParentID, strings.TrimSpace(body.Content)).Scan(&data)
	})
	if err != nil {
		return err
	}
	writeJSON(w, 201, data)
	return nil
}

func (s *server) reportPost(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "post")
	if err != nil {
		return err
	}
	var body struct {
		Reason string `json:"reason"`
		Detail string `json:"detail"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	if !clean(body.Reason, 1, 100) || !clean(body.Detail, 0, 1000) {
		return bad("invalid report")
	}
	if err := checkPost(r.Context(), s.db, user, id); err != nil {
		return err
	}
	if s.reports == "" {
		return apiError{503, "moderation report endpoint is not configured"}
	}
	payload, err := json.Marshal(map[string]any{"target_type": "post", "target_id": id,
		"reason": strings.TrimSpace(body.Reason), "detail": strings.TrimSpace(body.Detail)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.reports, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", r.Header.Get("Authorization"))
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return apiError{502, "moderation service unavailable"}
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		writeJSON(w, 202, map[string]string{"status": "submitted"})
		return nil
	case resp.StatusCode == 409:
		return conflict("post already reported")
	case resp.StatusCode == 400 || resp.StatusCode == 422:
		return bad("moderation service rejected the report")
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return forbidden()
	default:
		return apiError{502, "moderation service unavailable"}
	}
}

func isMissing(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
