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

CREATE TABLE groups (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id bigint NOT NULL REFERENCES users(id),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    description text NOT NULL CHECK (length(description) <= 1000),
    sport text NOT NULL CHECK (length(btrim(sport)) BETWEEN 1 AND 50),
    cover_url text,
    visibility text NOT NULL CHECK (visibility IN ('open','closed','private')),
    max_members integer NOT NULL CHECK (max_members BETWEEN 2 AND 500),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','disbanded')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX groups_active_name_idx ON groups (lower(name)) WHERE status = 'active';
CREATE TABLE group_members (
    group_id bigint NOT NULL REFERENCES groups(id),
    user_id bigint NOT NULL REFERENCES users(id),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id, group_id);
ALTER TABLE groups ADD CONSTRAINT groups_owner_membership
    FOREIGN KEY (id, owner_id) REFERENCES group_members(group_id,user_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE group_join_requests (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups(id),
    requester_id bigint NOT NULL REFERENCES users(id),
    reviewed_by bigint REFERENCES users(id),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    reviewed_at timestamptz,
    CHECK ((status = 'pending' AND reviewed_at IS NULL AND reviewed_by IS NULL)
       OR (status <> 'pending' AND reviewed_at IS NOT NULL AND reviewed_by IS NOT NULL))
);
CREATE UNIQUE INDEX pending_group_request_idx ON group_join_requests (group_id, requester_id) WHERE status = 'pending';
CREATE TABLE group_invites (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups(id),
    inviter_id bigint NOT NULL REFERENCES users(id),
    invitee_id bigint NOT NULL REFERENCES users(id),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','declined')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (inviter_id <> invitee_id)
);
CREATE UNIQUE INDEX pending_group_invite_idx ON group_invites(group_id,invitee_id) WHERE status = 'pending';
CREATE INDEX group_invites_user_idx ON group_invites(invitee_id,id DESC);
CREATE TABLE group_messages (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups(id),
    sender_id bigint NOT NULL REFERENCES users(id),
    content text NOT NULL CHECK (length(btrim(content)) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX group_messages_feed_idx ON group_messages (group_id, id DESC);
CREATE TABLE group_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups(id),
    created_by bigint NOT NULL REFERENCES users(id),
    title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 150),
    sport text NOT NULL CHECK (length(btrim(sport)) BETWEEN 1 AND 50),
    starts_at timestamptz NOT NULL,
    location text NOT NULL CHECK (length(btrim(location)) BETWEEN 1 AND 255),
    status text NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (group_id,id)
);
CREATE INDEX group_events_list_idx ON group_events (group_id, starts_at);
CREATE TABLE event_rsvps (
    event_id bigint NOT NULL,
    group_id bigint NOT NULL,
    user_id bigint NOT NULL,
    response text NOT NULL CHECK (response IN ('going','not_going')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id,user_id),
    FOREIGN KEY (group_id,event_id) REFERENCES group_events(group_id,id) ON DELETE CASCADE,
    FOREIGN KEY (group_id,user_id) REFERENCES group_members(group_id,user_id) ON DELETE CASCADE
);
CREATE TABLE group_notifications (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id),
    group_id bigint NOT NULL REFERENCES groups(id),
    kind text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz
);
CREATE INDEX group_notifications_inbox_idx ON group_notifications (user_id,id DESC);
