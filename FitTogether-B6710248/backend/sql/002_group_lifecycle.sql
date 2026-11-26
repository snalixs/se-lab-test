-- Existing groups keep their data. Owners must supply a schedule before writes.
ALTER TABLE groups
    ADD COLUMN starts_at timestamptz,
    ADD COLUMN ends_at timestamptz,
    ADD COLUMN location text,
    ADD COLUMN closed_at timestamptz,
    ADD COLUMN completed_at timestamptz,
    ADD COLUMN cancelled_at timestamptz,
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
-- The exact cancellation time of old groups is unknown; do not invent it.
ALTER TABLE groups DROP CONSTRAINT groups_status_check;
ALTER TABLE groups ADD CONSTRAINT groups_status_check
    CHECK (status IN ('active','completed','disbanded'));
ALTER TABLE groups DROP CONSTRAINT groups_visibility_check;
-- NOT VALID preserves old private groups without making them public.
-- New/updated rows must use the new contract.
ALTER TABLE groups ADD CONSTRAINT groups_visibility_check
    CHECK (visibility IN ('open','closed')) NOT VALID;
ALTER TABLE groups ADD CONSTRAINT groups_schedule_check CHECK (
    starts_at IS NOT NULL AND ends_at IS NOT NULL AND location IS NOT NULL
    AND ends_at > starts_at AND length(btrim(location)) BETWEEN 1 AND 255
) NOT VALID;
ALTER TABLE groups ADD CONSTRAINT groups_lifecycle_check CHECK (
    (status='active' AND completed_at IS NULL AND cancelled_at IS NULL)
    OR (status='completed' AND completed_at IS NOT NULL AND cancelled_at IS NULL)
    OR (status='disbanded' AND completed_at IS NULL AND cancelled_at IS NOT NULL)
) NOT VALID;

-- Archive the old multi-event model, preserving its data for explicit migration.
ALTER TABLE group_events RENAME TO legacy_group_events;
ALTER TABLE event_rsvps RENAME TO legacy_event_rsvps;

ALTER TABLE group_members DROP CONSTRAINT group_members_group_id_fkey;
ALTER TABLE group_members ADD FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE group_join_requests DROP CONSTRAINT group_join_requests_group_id_fkey;
ALTER TABLE group_join_requests ADD FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE group_invites DROP CONSTRAINT group_invites_group_id_fkey;
ALTER TABLE group_invites ADD FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE group_messages DROP CONSTRAINT group_messages_group_id_fkey;
ALTER TABLE group_messages ADD FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
ALTER TABLE group_notifications DROP CONSTRAINT group_notifications_group_id_fkey;
ALTER TABLE group_notifications ADD FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
-- Archived event data must not block lifecycle deletion of its parent group.
ALTER TABLE legacy_group_events DROP CONSTRAINT group_events_group_id_fkey;
ALTER TABLE legacy_group_events ADD FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;

CREATE INDEX groups_auto_complete_idx ON groups (ends_at) WHERE status='active';
CREATE INDEX groups_cleanup_idx ON groups (completed_at) WHERE status='completed';
CREATE INDEX groups_cancel_cleanup_idx ON groups (cancelled_at) WHERE status='disbanded';
CREATE FUNCTION set_group_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at := statement_timestamp();
    RETURN NEW;
END;
$$;
CREATE TRIGGER groups_updated_at BEFORE UPDATE ON groups
    FOR EACH ROW EXECUTE FUNCTION set_group_updated_at();
