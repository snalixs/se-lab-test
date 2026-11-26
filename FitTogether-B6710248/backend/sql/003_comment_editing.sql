ALTER TABLE comments ADD COLUMN updated_at timestamptz;
-- Deleting one's comment must not delete replies written by other members.
ALTER TABLE comments DROP CONSTRAINT comments_post_id_parent_comment_id_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_post_id_parent_comment_id_fkey
    FOREIGN KEY (post_id,parent_comment_id) REFERENCES comments(post_id,id)
    ON DELETE SET NULL (parent_comment_id);
