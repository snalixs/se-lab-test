-- ระบบโพสต์: PostgreSQL schema สำหรับฐานข้อมูลใหม่
-- ต้องมี users(id bigint PRIMARY KEY) จากระบบสมาชิกก่อนรัน
-- ไม่ใช่ migration สำหรับฐานข้อมูลที่รัน 001_features.sql แล้ว
-- user_follows รองรับฟีดผู้ติดตาม หากทีมมีตารางนี้แล้วต้องใช้ตารางร่วมกัน
-- PostReport ยังส่งไปบริการภายนอกตาม Backend ปัจจุบัน จึงไม่มีตารางรายงานที่นี่
BEGIN;

CREATE TABLE user_follows (
    follower_id bigint NOT NULL REFERENCES users(id),
    followed_id bigint NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followed_id),
    CHECK (follower_id <> followed_id)
);
CREATE TABLE posts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    author_id bigint NOT NULL REFERENCES users(id),
    shared_post_id bigint REFERENCES posts(id) ON DELETE SET NULL,
    content text NOT NULL CHECK (length(content) <= 1500),
    visibility text NOT NULL CHECK (visibility IN ('public','followers')),
    status text NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden','suspended','deleted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (length(btrim(content)) > 0 OR shared_post_id IS NOT NULL OR status = 'deleted')
);
CREATE INDEX posts_feed_idx ON posts (id DESC) WHERE status = 'published';
CREATE INDEX posts_author_idx ON posts (author_id, id DESC);
CREATE TABLE post_images (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id bigint NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    image_url text NOT NULL,
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    UNIQUE (post_id, sort_order)
);
CREATE TABLE hashtags (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 60)
);
CREATE TABLE post_hashtags (
    post_id bigint NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    hashtag_id bigint NOT NULL REFERENCES hashtags(id),
    PRIMARY KEY (post_id, hashtag_id)
);
CREATE INDEX post_hashtags_tag_idx ON post_hashtags (hashtag_id, post_id);
CREATE TABLE comments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post_id bigint NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_id bigint NOT NULL REFERENCES users(id),
    parent_comment_id bigint,
    content text NOT NULL CHECK (length(btrim(content)) BETWEEN 1 AND 350),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (post_id, id),
    FOREIGN KEY (post_id, parent_comment_id) REFERENCES comments(post_id, id) ON DELETE CASCADE
);
CREATE TABLE post_likes (
    post_id bigint NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);
CREATE TABLE post_bookmarks (
    post_id bigint NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);
CREATE INDEX post_bookmarks_user_idx ON post_bookmarks (user_id, post_id);

ALTER TABLE comments ADD COLUMN updated_at timestamptz;
-- Deleting one's comment must not delete replies written by other members.
ALTER TABLE comments DROP CONSTRAINT comments_post_id_parent_comment_id_fkey;
ALTER TABLE comments ADD CONSTRAINT comments_post_id_parent_comment_id_fkey
    FOREIGN KEY (post_id,parent_comment_id) REFERENCES comments(post_id,id)
    ON DELETE SET NULL (parent_comment_id);

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

COMMIT;
