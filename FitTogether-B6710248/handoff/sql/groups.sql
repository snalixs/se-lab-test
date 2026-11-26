-- ระบบกลุ่ม: หนึ่งกลุ่ม = หนึ่งกิจกรรม สมาชิก = ผู้เข้าร่วม (รวมเจ้าของ)
-- PostgreSQL schema สำหรับฐานข้อมูลใหม่ ต้องมี users(id bigint PRIMARY KEY) ก่อน
-- ไม่ใช่ migration สำหรับฐานข้อมูลเดิม และ Backend เดิมยังต้องปรับให้รองรับ
-- open: เข้าร่วมทันที; closed: อนุมัติคำขอหรือตอบรับคำเชิญ
-- closed_at คือเวลาปิดรับสมาชิก ไม่ใช่เวลาจบกิจกรรม
-- เจ้าของกดจบ: completed_at = เวลาที่กด
-- จบอัตโนมัติ: completed_at = ends_at + interval '24 hours'
-- ลบกลุ่มภายใน 24 ชั่วโมงหลัง completed_at โดย worker (ไฟล์นี้ไม่ได้สร้าง scheduler)
-- ยกเลิก: บันทึก cancelled_at และลบหลัง 24 ชั่วโมง; หลังจบปกติยังแชตได้จนลบ
-- สร้างกลุ่มและเพิ่มเจ้าของใน group_members ภายใน transaction เดียวกัน
-- API ต้องตรวจความจุ เวลาปิดรับ สิทธิ์ และสถานะ ก่อนเพิ่มสมาชิกทุกช่องทาง
-- ไม่มี setid, responded_at, group_events หรือ event_rsvps ในแบบนี้
BEGIN;

CREATE TABLE groups (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id bigint NOT NULL REFERENCES users(id),
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    description text NOT NULL CHECK (length(description) <= 1000),
    sport text NOT NULL CHECK (length(btrim(sport)) BETWEEN 1 AND 50),
    cover_url text,
    visibility text NOT NULL CHECK (visibility IN ('open','closed')),
    max_members integer NOT NULL CHECK (max_members BETWEEN 2 AND 500),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    location text NOT NULL CHECK (length(btrim(location)) BETWEEN 1 AND 255),
    closed_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','completed','disbanded')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at),
    CHECK (
        (status = 'active' AND completed_at IS NULL AND cancelled_at IS NULL)
        OR (status = 'completed' AND completed_at IS NOT NULL AND cancelled_at IS NULL)
        OR (status = 'disbanded' AND completed_at IS NULL AND cancelled_at IS NOT NULL)
    )
);
CREATE UNIQUE INDEX groups_active_name_idx ON groups (lower(name)) WHERE status = 'active';
CREATE TABLE group_members (
    group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id bigint NOT NULL REFERENCES users(id),
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id, group_id);
ALTER TABLE groups ADD CONSTRAINT groups_owner_membership
    FOREIGN KEY (id, owner_id) REFERENCES group_members(group_id,user_id) DEFERRABLE INITIALLY DEFERRED;
CREATE TABLE group_join_requests (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
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
    group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
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
    group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    sender_id bigint NOT NULL REFERENCES users(id),
    content text NOT NULL CHECK (length(btrim(content)) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX group_messages_feed_idx ON group_messages (group_id, id DESC);
CREATE TABLE group_notifications (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id),
    group_id bigint NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    kind text NOT NULL,
    data jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    read_at timestamptz
);
CREATE INDEX group_notifications_inbox_idx ON group_notifications (user_id,id DESC);

-- Index สำหรับ worker จบอัตโนมัติและลบกลุ่ม
CREATE INDEX groups_auto_complete_idx ON groups (ends_at) WHERE status = 'active';
CREATE INDEX groups_cleanup_idx ON groups (completed_at) WHERE status = 'completed';
CREATE INDEX groups_cancel_cleanup_idx ON groups (cancelled_at) WHERE status = 'disbanded';

-- อัปเดตเวลาทุกครั้งที่แก้แถวข้อมูลกลุ่ม
CREATE FUNCTION set_group_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := statement_timestamp();
    RETURN NEW;
END;
$$;
CREATE TRIGGER groups_updated_at
    BEFORE UPDATE ON groups
    FOR EACH ROW EXECUTE FUNCTION set_group_updated_at();

COMMIT;
