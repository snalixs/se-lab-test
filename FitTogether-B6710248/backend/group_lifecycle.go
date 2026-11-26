package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const groupNotExpiredSQL = `NOT (
 (g.completed_at IS NOT NULL AND g.completed_at + interval '24 hours' <= statement_timestamp())
 OR (g.cancelled_at IS NOT NULL AND g.cancelled_at + interval '24 hours' <= statement_timestamp())
 OR (g.status='active' AND g.ends_at IS NOT NULL AND g.ends_at + interval '48 hours' <= statement_timestamp())
)`

func (g groupData) completionTime() *time.Time {
	if g.completedAt != nil {
		return g.completedAt
	}
	if g.status == "active" && g.endsAt != nil {
		due := g.endsAt.Add(24 * time.Hour)
		if !g.now.Before(due) {
			return &due
		}
	}
	return nil
}

func (g groupData) expired() bool {
	ended := g.completionTime()
	if g.cancelledAt != nil {
		ended = g.cancelledAt
	}
	return ended != nil && !g.now.Before(ended.Add(24*time.Hour))
}

func (g groupData) active() error {
	if g.status != "active" || g.completionTime() != nil {
		return conflict("activity has already ended")
	}
	return nil
}

func (g groupData) admissionAllowed() error {
	if err := g.active(); err != nil {
		return err
	}
	if g.startsAt == nil || g.endsAt == nil || g.visibility == "private" {
		return conflict("owner must configure the activity schedule and visibility first")
	}
	if g.closedAt != nil && !g.now.Before(*g.closedAt) {
		return conflict("registration is closed")
	}
	return nil
}

func retiredGroupEvents(http.ResponseWriter, *http.Request, int64) error {
	return apiError{410, "each group is now one activity; use groups and group membership instead of events/RSVP"}
}

func (s *server) completeGroup(w http.ResponseWriter, r *http.Request, user int64) error {
	return s.finishGroup(w, r, user, false)
}

func (s *server) finishGroup(w http.ResponseWriter, r *http.Request, user int64, cancel bool) error {
	err := s.withGroup(r, "FOR UPDATE", func(ctx context.Context, tx pgx.Tx, g groupData) error {
		if g.owner != user {
			return forbidden()
		}
		target := "completed"
		if cancel {
			target = "disbanded"
		}
		if g.status == target {
			return nil
		}
		if g.status != "active" {
			return conflict("activity has already ended")
		}
		if g.startsAt == nil || g.endsAt == nil || g.visibility == "private" {
			return conflict("configure the activity first")
		}
		if cancel && g.completionTime() != nil {
			return conflict("activity has already ended")
		}
		if !cancel && g.now.Before(*g.startsAt) {
			return conflict("activity has not started; cancel it instead")
		}
		ended := g.now
		if due := g.completionTime(); due != nil {
			ended = *due
		}
		query := "UPDATE groups SET status='completed',completed_at=$2 WHERE id=$1"
		kind := "group_completed"
		if cancel {
			query = "UPDATE groups SET status='disbanded',cancelled_at=$2 WHERE id=$1"
			kind = "group_disbanded"
		}
		if _, err := tx.Exec(ctx, query, g.id, ended); err != nil {
			return err
		}
		return notifyMembers(ctx, tx, g.id, 0, kind, map[string]any{"delete_at": ended.Add(24 * time.Hour)})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// Called on startup and periodically. Row locks serialize this with owner actions.
// Skip locked rows so one slow request does not block every expired group.
func (s *server) sweepGroups(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `WITH due AS (
    SELECT id FROM groups WHERE status='active' AND ends_at + interval '24 hours' <= statement_timestamp()
    ORDER BY ends_at LIMIT 200 FOR UPDATE SKIP LOCKED
   ), finished AS (
    UPDATE groups g SET status='completed',completed_at=g.ends_at + interval '24 hours'
    FROM due WHERE g.id=due.id RETURNING g.id,g.completed_at
   ) INSERT INTO group_notifications(group_id,user_id,kind,data)
   SELECT f.id,m.user_id,'group_completed',jsonb_build_object('delete_at', f.completed_at + interval '24 hours')
   FROM finished f JOIN group_members m ON m.group_id=f.id`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `WITH due AS (
   SELECT id FROM groups WHERE
    completed_at + interval '24 hours' <= statement_timestamp()
    OR cancelled_at + interval '24 hours' <= statement_timestamp()
   ORDER BY id LIMIT 200 FOR UPDATE SKIP LOCKED
  ) DELETE FROM groups g USING due WHERE g.id=due.id`)
		return err
	})
}

func (s *server) runGroupWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.sweepGroups(runCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("group lifecycle worker: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
