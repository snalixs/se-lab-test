package main

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

func (s *server) deletePost(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "post")
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var locked int64
		err := tx.QueryRow(ctx, "SELECT id FROM posts WHERE id=$1 AND author_id=$2 FOR UPDATE", id, user).Scan(&locked)
		if isMissing(err) {
			var alreadyDeleted bool
			if e := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM deleted_posts WHERE original_post_id=$1 AND author_id=$2)", id, user).Scan(&alreadyDeleted); e != nil {
				return e
			}
			if alreadyDeleted {
				return nil
			}
			return missing()
		}
		if err != nil {
			return err
		}
		// All live post interaction writers lock the post before changing child rows.
		// A snapshot and physical deletion either both commit or both roll back.
		_, err = tx.Exec(ctx, `INSERT INTO deleted_posts(original_post_id,author_id,deleted_by,post_data,related_data)
   SELECT p.id,p.author_id,$2,to_jsonb(p),jsonb_build_object(
    'images',COALESCE((SELECT jsonb_agg(to_jsonb(i) ORDER BY i.sort_order) FROM post_images i WHERE i.post_id=p.id),'[]'::jsonb),
    'comments',COALESCE((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM comments c WHERE c.post_id=p.id),'[]'::jsonb),
    'likes',COALESCE((SELECT jsonb_agg(to_jsonb(l) ORDER BY l.user_id) FROM post_likes l WHERE l.post_id=p.id),'[]'::jsonb),
    'bookmarks',COALESCE((SELECT jsonb_agg(to_jsonb(b) ORDER BY b.user_id) FROM post_bookmarks b WHERE b.post_id=p.id),'[]'::jsonb),
    'hashtags',COALESCE((SELECT jsonb_agg(jsonb_build_object('post_id',ph.post_id,'hashtag_id',h.id,'name',h.name) ORDER BY h.id)
      FROM post_hashtags ph JOIN hashtags h ON h.id=ph.hashtag_id WHERE ph.post_id=p.id),'[]'::jsonb)
   ) FROM posts p WHERE p.id=$1`, id, user)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE posts SET deleted_shared_post_id=$1,shared_post_id=NULL,updated_at=clock_timestamp()
    WHERE shared_post_id=$1`, id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "DELETE FROM posts WHERE id=$1", id)
		return err
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
