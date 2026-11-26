package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *server) messages(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	return s.withGroup(r, "FOR SHARE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if err := requireMember(ctx, tx, g.id, user); err != nil {
			return err
		}
		return listQuery(w, tx, ctx, `SELECT m.*,p.display_name,p.photo_url FROM group_messages m
			LEFT JOIN profiles p ON p.user_id=m.sender_id WHERE m.group_id=$1
			ORDER BY m.id DESC LIMIT $2 OFFSET $3`, g.id, limit, offset)
	})
}

func (s *server) sendMessage(w http.ResponseWriter, r *http.Request, user int64) error {
	var input struct {
		Content string `json:"content"`
	}
	if err := decode(w, r, &input); err != nil {
		return err
	}
	if !clean(input.Content, 1, 500) {
		return bad("content must be 1–500 characters")
	}
	var data json.RawMessage
	err := s.withGroup(r, "FOR SHARE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if err := requireMember(ctx, tx, g.id, user); err != nil {
			return err
		}
		if g.status == "disbanded" {
			return conflict("activity has been cancelled")
		}
		return tx.QueryRow(ctx, `WITH new_message AS (
			INSERT INTO group_messages (group_id,sender_id,content) VALUES ($1,$2,$3) RETURNING *
		) SELECT to_jsonb(new_message) FROM new_message`, g.id, user, strings.TrimSpace(input.Content)).Scan(&data)
	})
	if err != nil {
		return err
	}
	writeJSON(w, 201, data)
	return nil
}

func (s *server) notifications(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	return listQuery(w, s.db, r.Context(), `SELECT * FROM group_notifications WHERE user_id=$1
		ORDER BY id DESC LIMIT $2 OFFSET $3`, user, limit, offset)
}

func (s *server) markNotification(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "notification")
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(r.Context(), `UPDATE group_notifications SET read_at=COALESCE(read_at,now())
		WHERE id=$1 AND user_id=$2`, id, user)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return missing()
	}
	w.WriteHeader(204)
	return nil
}
