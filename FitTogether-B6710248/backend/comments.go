package main

import (
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Lock the parent first so editing, replying and post archival cannot race.
func (s *server) updateComment(w http.ResponseWriter, r *http.Request, user int64) error {
	post, err := idParam(r, "post")
	if err != nil {
		return err
	}
	comment, err := idParam(r, "comment")
	if err != nil {
		return err
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := decode(w, r, &body); err != nil {
		return err
	}
	body.Content = strings.TrimSpace(body.Content)
	if !clean(body.Content, 1, 350) {
		return bad("comment must contain 1–350 characters")
	}
	ctx := r.Context()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := checkPost(ctx, tx, user, post); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE comments SET content=$4,updated_at=clock_timestamp()
    WHERE id=$1 AND post_id=$2 AND author_id=$3`, comment, post, user, body.Content)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return missing()
		}
		return nil
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) deleteComment(w http.ResponseWriter, r *http.Request, user int64) error {
	post, err := idParam(r, "post")
	if err != nil {
		return err
	}
	comment, err := idParam(r, "comment")
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := checkPost(ctx, tx, user, post); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM comments WHERE id=$1 AND post_id=$2 AND author_id=$3", comment, post, user)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return missing()
		}
		return nil
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
