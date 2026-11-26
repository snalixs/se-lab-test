package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type groupData struct {
	id, owner                                            int64
	visibility                                           string
	capacity                                             int
	status                                               string
	startsAt, endsAt, closedAt, completedAt, cancelledAt *time.Time
	now                                                  time.Time
}

type groupInput struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Sport       string     `json:"sport"`
	CoverURL    *string    `json:"cover_url"`
	Visibility  string     `json:"visibility"`
	MaxMembers  int        `json:"max_members"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Location    string     `json:"location"`
}

func validGroup(g *groupInput) error {
	g.Name = strings.TrimSpace(g.Name)
	g.Sport = strings.TrimSpace(g.Sport)
	g.Description = strings.TrimSpace(g.Description)
	if !clean(g.Name, 1, 100) || !clean(g.Sport, 1, 50) || !clean(g.Description, 0, 1000) {
		return bad("invalid group name, sport, or description")
	}
	switch g.Visibility {
	case "open", "closed":
	default:
		return bad("visibility must be open or closed")
	}
	if g.MaxMembers < 2 || g.MaxMembers > 500 {
		return bad("max_members must be between 2 and 500")
	}
	if g.CoverURL != nil && !validMediaURL(*g.CoverURL) {
		return bad("cover_url must be HTTPS or null")
	}
	g.Location = strings.TrimSpace(g.Location)
	if g.StartsAt.IsZero() || g.EndsAt.IsZero() || !g.EndsAt.After(g.StartsAt) || !clean(g.Location, 1, 255) {
		return bad("starts_at, ends_at after starts_at, and location are required")
	}
	return nil
}

const groupSelect = `SELECT g.id,g.owner_id,g.name,g.description,g.sport,g.cover_url,g.visibility,g.max_members,
	g.created_at,g.updated_at,g.starts_at,g.ends_at,g.closed_at,g.location,g.cancelled_at,
	CASE WHEN g.status='active' AND g.ends_at + interval '24 hours' <= statement_timestamp()
	    THEN 'completed' ELSE g.status END AS status,
	COALESCE(g.completed_at, CASE WHEN g.status='active' AND g.ends_at + interval '24 hours' <= statement_timestamp()
	    THEN g.ends_at + interval '24 hours' END) AS completed_at,
	COALESCE(g.completed_at + interval '24 hours', g.cancelled_at + interval '24 hours',
	    CASE WHEN g.ends_at + interval '24 hours' <= statement_timestamp() THEN g.ends_at + interval '48 hours' END) AS delete_at,
	(g.starts_at IS NULL OR g.ends_at IS NULL OR g.location IS NULL OR g.visibility='private') AS needs_setup,
	COALESCE((g.status='active' AND g.visibility IN ('open','closed') AND g.starts_at IS NOT NULL
	 AND g.ends_at + interval '24 hours' > statement_timestamp()
	 AND (g.closed_at IS NULL OR g.closed_at > statement_timestamp())
	 AND (SELECT count(*) FROM group_members m WHERE m.group_id=g.id) < g.max_members),false) AS accepting_members,
	(SELECT count(*) FROM group_members m WHERE m.group_id=g.id) AS member_count,
	EXISTS (SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=$1) AS is_member,
	(SELECT status FROM group_join_requests j WHERE j.group_id=g.id AND j.requester_id=$1 ORDER BY j.id DESC LIMIT 1) AS request_status,
	(SELECT id FROM group_invites i WHERE i.group_id=g.id AND i.invitee_id=$1 AND i.status='pending') AS pending_invite_id
	FROM groups g WHERE ` + groupNotExpiredSQL + ` AND
	((g.status='active' AND g.visibility<>'private'
	  AND (g.ends_at IS NULL OR g.ends_at + interval '24 hours' > statement_timestamp())) OR EXISTS (
		SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=$1))`

func (s *server) groups(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	search, sport, visibility, scope := strings.TrimSpace(q.Get("q")), strings.TrimSpace(q.Get("sport")), q.Get("visibility"), q.Get("scope")
	if !clean(search, 0, 100) || !clean(sport, 0, 50) {
		return bad("invalid group filter")
	}
	if visibility != "" && visibility != "open" && visibility != "closed" {
		return bad("invalid group visibility filter")
	}
	if scope != "" && scope != "mine" {
		return bad("scope must be mine or omitted")
	}
	return listQuery(w, s.db, r.Context(), groupSelect+`
		AND ($2::text='' OR strpos(lower(g.name),lower($2))>0 OR strpos(lower(g.sport),lower($2))>0)
		AND ($3::text='' OR lower(g.sport)=lower($3))
		AND ($4::text='' OR g.visibility=$4)
		AND ($5::text<>'mine' OR EXISTS (SELECT 1 FROM group_members m WHERE m.group_id=g.id AND m.user_id=$1))
		AND ($5::text='mine' OR (g.visibility <> 'private' AND g.status='active'
		    AND (g.ends_at IS NULL OR g.ends_at + interval '24 hours' > statement_timestamp())))
		ORDER BY g.id DESC LIMIT $6 OFFSET $7`, user, search, sport, visibility, scope, limit, offset)
}

func (s *server) getGroup(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "group")
	if err != nil {
		return err
	}
	return s.groupResponse(w, r.Context(), user, id, 200)
}

func (s *server) groupResponse(w http.ResponseWriter, ctx context.Context, user, id int64, status int) error {
	return jsonRow(w, s.db, ctx, status, "SELECT to_jsonb(record) FROM ("+groupSelect+" AND g.id=$2) record", user, id)
}

func (s *server) createGroup(w http.ResponseWriter, r *http.Request, user int64) error {
	var input groupInput
	if err := decode(w, r, &input); err != nil {
		return err
	}
	if err := validGroup(&input); err != nil {
		return err
	}
	if !input.StartsAt.After(time.Now()) {
		return bad("starts_at must be in the future")
	}
	ctx := r.Context()
	var id int64
	err := pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO groups(owner_id,name,description,sport,cover_url,visibility,max_members,starts_at,ends_at,closed_at,location)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, user, input.Name, input.Description, input.Sport, input.CoverURL, input.Visibility, input.MaxMembers, input.StartsAt, input.EndsAt, input.ClosedAt, input.Location).Scan(&id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO group_members(group_id,user_id) VALUES ($1,$2)", id, user)
		return err
	})
	if err != nil {
		return err
	}
	return s.groupResponse(w, ctx, user, id, 201)
}

// Serialize membership and capacity changes on the group's row. Readers using a
// shared lock see a consistent authorization decision until their query finishes.
func (s *server) withGroup(r *http.Request, lock string, fn func(context.Context, pgx.Tx, groupData) error) error {
	id, err := idParam(r, "group")
	if err != nil {
		return err
	}
	ctx := r.Context()
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		g := groupData{id: id}
		err := tx.QueryRow(ctx, "SELECT owner_id,visibility,max_members,status,starts_at,ends_at,closed_at,completed_at,cancelled_at FROM groups WHERE id=$1 "+lock, id).Scan(&g.owner, &g.visibility, &g.capacity, &g.status, &g.startsAt, &g.endsAt, &g.closedAt, &g.completedAt, &g.cancelledAt)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&g.now); err != nil {
			return err
		}
		if g.expired() {
			return missing()
		}
		return fn(ctx, tx, g)
	})
}

func memberExists(ctx context.Context, q dbReader, group, user int64) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND user_id=$2)", group, user).Scan(&found)
	return found, err
}

func requireMember(ctx context.Context, q dbReader, group, user int64) error {
	ok, err := memberExists(ctx, q, group, user)
	if err != nil {
		return err
	}
	if !ok {
		return missing()
	}
	return nil
}

func addMember(ctx context.Context, tx pgx.Tx, g groupData, user int64) error {
	if err := g.admissionAllowed(); err != nil {
		return err
	}
	member, err := memberExists(ctx, tx, g.id, user)
	if err != nil || member {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM group_members WHERE group_id=$1", g.id).Scan(&count); err != nil {
		return err
	}
	if count >= g.capacity {
		return conflict("group is full")
	}
	_, err = tx.Exec(ctx, "INSERT INTO group_members(group_id,user_id) VALUES ($1,$2)", g.id, user)
	return err
}

func notifyOne(ctx context.Context, tx pgx.Tx, group, user int64, kind string, data map[string]any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO group_notifications(group_id,user_id,kind,data) VALUES ($1,$2,$3,$4)", group, user, kind, payload)
	return err
}

func notifyMembers(ctx context.Context, tx pgx.Tx, group, exclude int64, kind string, data map[string]any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO group_notifications(group_id,user_id,kind,data)
		SELECT group_id,user_id,$3,$4 FROM group_members WHERE group_id=$1 AND user_id<>$2`, group, exclude, kind, payload)
	return err
}

func (s *server) updateGroup(w http.ResponseWriter, r *http.Request, user int64) error {
	var patch struct {
		Name        *string         `json:"name"`
		Description *string         `json:"description"`
		Sport       *string         `json:"sport"`
		CoverURL    json.RawMessage `json:"cover_url"`
		Visibility  *string         `json:"visibility"`
		MaxMembers  *int            `json:"max_members"`
		StartsAt    *time.Time      `json:"starts_at"`
		EndsAt      *time.Time      `json:"ends_at"`
		ClosedAt    json.RawMessage `json:"closed_at"`
		Location    *string         `json:"location"`
	}
	if err := decode(w, r, &patch); err != nil {
		return err
	}
	err := s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if g.owner != user {
			return forbidden()
		}
		if err := g.active(); err != nil {
			return err
		}
		input := groupInput{Visibility: g.visibility, MaxMembers: g.capacity}
		if g.startsAt != nil {
			input.StartsAt = *g.startsAt
		}
		if g.endsAt != nil {
			input.EndsAt = *g.endsAt
		}
		input.ClosedAt = g.closedAt
		err := tx.QueryRow(ctx, "SELECT name,description,sport,cover_url,COALESCE(location,'') FROM groups WHERE id=$1", g.id).Scan(&input.Name, &input.Description, &input.Sport, &input.CoverURL, &input.Location)
		if err != nil {
			return err
		}
		if patch.Name != nil {
			input.Name = *patch.Name
		}
		if patch.Description != nil {
			input.Description = *patch.Description
		}
		if patch.Sport != nil {
			input.Sport = *patch.Sport
		}
		if patch.CoverURL != nil && json.Unmarshal(patch.CoverURL, &input.CoverURL) != nil {
			return bad("cover_url must be a string or null")
		}
		if patch.Visibility != nil {
			input.Visibility = *patch.Visibility
		}
		if patch.MaxMembers != nil {
			input.MaxMembers = *patch.MaxMembers
		}
		if patch.StartsAt != nil {
			input.StartsAt = *patch.StartsAt
		}
		if patch.EndsAt != nil {
			input.EndsAt = *patch.EndsAt
		}
		if patch.Location != nil {
			input.Location = *patch.Location
		}
		if patch.ClosedAt != nil && json.Unmarshal(patch.ClosedAt, &input.ClosedAt) != nil {
			return bad("closed_at must be RFC3339 or null")
		}
		if err := validGroup(&input); err != nil {
			return err
		}
		if !input.EndsAt.Add(24 * time.Hour).After(g.now) {
			return bad("activity is already past its automatic completion deadline")
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM group_members WHERE group_id=$1", g.id).Scan(&count); err != nil {
			return err
		}
		if input.MaxMembers < count {
			return conflict("max_members is below the current member count")
		}
		if _, err := tx.Exec(ctx, `UPDATE groups SET name=$2,description=$3,sport=$4,cover_url=$5,
			visibility=$6,max_members=$7,starts_at=$8,ends_at=$9,closed_at=$10,location=$11 WHERE id=$1`, g.id, input.Name, input.Description, input.Sport, input.CoverURL, input.Visibility, input.MaxMembers, input.StartsAt, input.EndsAt, input.ClosedAt, input.Location); err != nil {
			return err
		}
		if err := notifyMembers(ctx, tx, g.id, user, "group_updated", map[string]any{}); err != nil {
			return err
		}
		if input.Visibility != "closed" && g.visibility == "closed" {
			if _, err := tx.Exec(ctx, `INSERT INTO group_notifications(group_id,user_id,kind,data)
				SELECT group_id,requester_id,'join_rejected',jsonb_build_object('request_id',id)
				FROM group_join_requests WHERE group_id=$1 AND status='pending'`, g.id); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE group_join_requests SET status='rejected',reviewed_by=$2,reviewed_at=now()
				WHERE group_id=$1 AND status='pending'`, g.id, user)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}

func (s *server) disband(w http.ResponseWriter, r *http.Request, user int64) error {
	return s.finishGroup(w, r, user, true)
}

func (s *server) joinGroup(w http.ResponseWriter, r *http.Request, user int64) error {
	result := map[string]any{"status": "joined"}
	err := s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if err := g.admissionAllowed(); err != nil {
			return err
		}
		member, err := memberExists(ctx, tx, g.id, user)
		if err != nil || member {
			return err
		}
		switch g.visibility {
		case "open":
			if err := addMember(ctx, tx, g, user); err != nil {
				return err
			}
			return completeAdmissions(ctx, tx, g.id, user, g.owner)
		}
		var id int64
		err = tx.QueryRow(ctx, `INSERT INTO group_join_requests(group_id,requester_id) VALUES ($1,$2)
			ON CONFLICT (group_id,requester_id) WHERE status='pending' DO NOTHING RETURNING id`, g.id, user).Scan(&id)
		if isMissing(err) {
			err = tx.QueryRow(ctx, "SELECT id FROM group_join_requests WHERE group_id=$1 AND requester_id=$2 AND status='pending'", g.id, user).Scan(&id)
		} else if err == nil {
			err = notifyOne(ctx, tx, g.id, g.owner, "join_requested", map[string]any{"request_id": id, "requester_id": user})
		}
		if err != nil {
			return err
		}
		result["status"], result["request_id"] = "pending", id
		return nil
	})
	if err != nil {
		return err
	}
	status := 200
	if result["status"] == "pending" {
		status = 202
	}
	writeJSON(w, status, result)
	return nil
}

func completeAdmissions(ctx context.Context, tx pgx.Tx, group, user, reviewer int64) error {
	if _, err := tx.Exec(ctx, `UPDATE group_invites SET status='accepted' WHERE group_id=$1 AND invitee_id=$2 AND status='pending'`, group, user); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE group_join_requests SET status='approved',reviewed_by=$3,reviewed_at=now()
		WHERE group_id=$1 AND requester_id=$2 AND status='pending'`, group, user, reviewer)
	return err
}

func (s *server) joinRequests(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	return s.withGroup(r, "FOR SHARE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if g.owner != user {
			return forbidden()
		}
		return listQuery(w, tx, ctx, `SELECT j.*,p.display_name,p.photo_url FROM group_join_requests j
			LEFT JOIN profiles p ON p.user_id=j.requester_id WHERE j.group_id=$1
			ORDER BY (j.status='pending') DESC,j.id DESC LIMIT $2 OFFSET $3`, g.id, limit, offset)
	})
}

func (s *server) reviewRequest(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "request")
	if err != nil {
		return err
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &input); err != nil {
		return err
	}
	if input.Status != "approved" && input.Status != "rejected" {
		return bad("status must be approved or rejected")
	}
	err = s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if g.owner != user {
			return forbidden()
		}
		if err := g.active(); err != nil {
			return err
		}
		var member int64
		var status string
		if err := tx.QueryRow(ctx, "SELECT requester_id,status FROM group_join_requests WHERE id=$1 AND group_id=$2", id, g.id).Scan(&member, &status); err != nil {
			return err
		}
		if status == input.Status {
			return nil
		}
		if status != "pending" {
			return conflict("request has already been reviewed")
		}
		if input.Status == "approved" {
			if g.visibility != "closed" {
				return conflict("group no longer accepts join requests")
			}
			if err := addMember(ctx, tx, g, member); err != nil {
				return err
			}
			if err := completeAdmissions(ctx, tx, g.id, member, user); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `UPDATE group_join_requests SET status='rejected',reviewed_by=$2,reviewed_at=now() WHERE id=$1`, id, user); err != nil {
				return err
			}
		}
		return notifyOne(ctx, tx, g.id, member, "join_"+input.Status, map[string]any{"request_id": id})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}

func (s *server) invite(w http.ResponseWriter, r *http.Request, user int64) error {
	var input struct {
		UserID int64 `json:"user_id"`
	}
	if err := decode(w, r, &input); err != nil {
		return err
	}
	if input.UserID <= 0 || input.UserID == user {
		return bad("invalid invitee")
	}
	var id int64
	status := 201
	err := s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if g.owner != user {
			return forbidden()
		}
		if err := g.admissionAllowed(); err != nil {
			return err
		}
		if g.visibility != "closed" {
			return conflict("invitations are only for closed groups")
		}
		member, err := memberExists(ctx, tx, g.id, input.UserID)
		if err != nil {
			return err
		}
		if member {
			return conflict("user is already a member")
		}
		err = tx.QueryRow(ctx, `INSERT INTO group_invites(group_id,inviter_id,invitee_id) VALUES ($1,$2,$3)
			ON CONFLICT (group_id,invitee_id) WHERE status='pending' DO NOTHING RETURNING id`, g.id, user, input.UserID).Scan(&id)
		if isMissing(err) {
			status = 200
			return tx.QueryRow(ctx, "SELECT id FROM group_invites WHERE group_id=$1 AND invitee_id=$2 AND status='pending'", g.id, input.UserID).Scan(&id)
		}
		if err != nil {
			return err
		}
		return notifyOne(ctx, tx, g.id, input.UserID, "group_invited", map[string]any{"invite_id": id, "inviter_id": user})
	})
	if err != nil {
		return err
	}
	writeJSON(w, status, map[string]any{"id": id, "status": "pending"})
	return nil
}

func (s *server) myInvites(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	return listQuery(w, s.db, r.Context(), `SELECT i.*,g.name AS group_name FROM group_invites i JOIN groups g ON g.id=i.group_id
		WHERE i.invitee_id=$1 AND g.status='active' AND g.visibility='closed'
        AND g.ends_at + interval '24 hours' > statement_timestamp()
        AND (g.closed_at IS NULL OR g.closed_at > statement_timestamp()) ORDER BY i.id DESC LIMIT $2 OFFSET $3`, user, limit, offset)
}

func (s *server) reviewInvite(w http.ResponseWriter, r *http.Request, user int64) error {
	id, err := idParam(r, "invite")
	if err != nil {
		return err
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := decode(w, r, &input); err != nil {
		return err
	}
	if input.Status != "accepted" && input.Status != "declined" {
		return bad("status must be accepted or declined")
	}
	err = s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if err := g.active(); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, "SELECT status FROM group_invites WHERE id=$1 AND group_id=$2 AND invitee_id=$3", id, g.id, user).Scan(&status); err != nil {
			return err
		}
		if status == input.Status {
			return nil
		}
		if status != "pending" {
			return conflict("invitation has already been answered")
		}
		if input.Status == "accepted" {
			if g.visibility != "closed" {
				return conflict("group no longer accepts invitations")
			}
			if err := addMember(ctx, tx, g, user); err != nil {
				return err
			}
			if err := completeAdmissions(ctx, tx, g.id, user, g.owner); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, "UPDATE group_invites SET status='declined' WHERE id=$1", id); err != nil {
			return err
		}
		return notifyOne(ctx, tx, g.id, g.owner, "invite_"+input.Status, map[string]any{"invite_id": id, "user_id": user})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}

func (s *server) members(w http.ResponseWriter, r *http.Request, user int64) error {
	limit, offset, err := paging(r)
	if err != nil {
		return err
	}
	return s.withGroup(r, "FOR SHARE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if err := requireMember(ctx, tx, g.id, user); err != nil {
			return err
		}
		return listQuery(w, tx, ctx, `SELECT m.user_id,m.joined_at,(m.user_id=$2) AS is_owner,p.display_name,p.photo_url
			FROM group_members m LEFT JOIN profiles p ON p.user_id=m.user_id WHERE m.group_id=$1
			ORDER BY (m.user_id=$2) DESC,m.joined_at,m.user_id LIMIT $3 OFFSET $4`, g.id, g.owner, limit, offset)
	})
}

func (s *server) leaveGroup(w http.ResponseWriter, r *http.Request, user int64) error {
	return s.deleteMember(w, r, user, user)
}

func (s *server) removeMember(w http.ResponseWriter, r *http.Request, user int64) error {
	target, err := idParam(r, "user")
	if err != nil {
		return err
	}
	return s.deleteMember(w, r, user, target)
}

func (s *server) deleteMember(w http.ResponseWriter, r *http.Request, user, target int64) error {
	err := s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if user != target && g.owner != user {
			return forbidden()
		}
		if err := g.active(); err != nil {
			return err
		}
		if target == g.owner {
			return conflict("transfer ownership before leaving the group")
		}
		tag, err := tx.Exec(ctx, "DELETE FROM group_members WHERE group_id=$1 AND user_id=$2", g.id, target)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return missing()
		}
		if user != target {
			return notifyOne(ctx, tx, g.id, target, "removed_from_group", map[string]any{})
		}
		return nil
	})
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}

func (s *server) transfer(w http.ResponseWriter, r *http.Request, user int64) error {
	var input struct {
		UserID int64 `json:"user_id"`
	}
	if err := decode(w, r, &input); err != nil {
		return err
	}
	if input.UserID <= 0 || input.UserID == user {
		return bad("choose another existing group member")
	}
	err := s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if g.owner != user {
			return forbidden()
		}
		if err := g.active(); err != nil {
			return err
		}
		ok, err := memberExists(ctx, tx, g.id, input.UserID)
		if err != nil {
			return err
		}
		if !ok {
			return conflict("new owner must already be a member")
		}
		if _, err := tx.Exec(ctx, "UPDATE groups SET owner_id=$2 WHERE id=$1", g.id, input.UserID); err != nil {
			return err
		}
		return notifyOne(ctx, tx, g.id, input.UserID, "ownership_transferred", map[string]any{"previous_owner_id": user})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}
