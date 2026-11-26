-- Historical IDs intentionally have no foreign keys to live tables.
CREATE TABLE deleted_posts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    original_post_id bigint NOT NULL UNIQUE,
    author_id bigint NOT NULL,
    deleted_by bigint NOT NULL,
    deleted_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    snapshot_version integer NOT NULL DEFAULT 1,
    post_data jsonb NOT NULL CHECK (jsonb_typeof(post_data)='object'),
    related_data jsonb NOT NULL CHECK (jsonb_typeof(related_data)='object')
);
CREATE INDEX deleted_posts_author_idx ON deleted_posts(author_id,deleted_at DESC);

-- Preserve share captions/interactions without exposing the archived original.
ALTER TABLE posts ADD COLUMN deleted_shared_post_id bigint;
ALTER TABLE posts DROP CONSTRAINT posts_check;
ALTER TABLE posts ADD CONSTRAINT posts_content_required CHECK (
    length(btrim(content)) > 0 OR shared_post_id IS NOT NULL
    OR deleted_shared_post_id IS NOT NULL OR status='deleted'
);
ALTER TABLE posts ADD CONSTRAINT posts_share_reference_check CHECK (
    shared_post_id IS NULL OR deleted_shared_post_id IS NULL
);
