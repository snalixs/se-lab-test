package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGroupTimeRules(t *testing.T) {
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	start, end := now.Add(-48*time.Hour), now.Add(24*time.Hour)
	g := groupData{status: "active", visibility: "open", startsAt: &start, endsAt: &end, now: now}
	if err := g.admissionAllowed(); err != nil {
		t.Fatal("multi-day activity ended prematurely:", err)
	}
	closeAt := now
	g.closedAt = &closeAt
	if g.admissionAllowed() == nil {
		t.Fatal("registration must close at the exact deadline")
	}
	g.closedAt = nil
	g.now = end.Add(24 * time.Hour)
	if g.completionTime() == nil || !g.completionTime().Equal(g.now) {
		t.Fatal("automatic completion deadline")
	}
	if g.admissionAllowed() == nil {
		t.Fatal("completed activity accepted admission")
	}
	if g.expired() {
		t.Fatal("group deleted without its 24-hour retention")
	}
	g.now = end.Add(48 * time.Hour)
	if !g.expired() {
		t.Fatal("group must expire at the retention deadline")
	}
	cancelled := now.Add(-24 * time.Hour)
	g = groupData{status: "disbanded", cancelledAt: &cancelled, now: now}
	if !g.expired() {
		t.Fatal("cancelled group did not expire")
	}
}

func TestGroupValidation(t *testing.T) {
	now := time.Now().UTC()
	base := groupInput{Name: "Run", Sport: "running", Visibility: "closed", MaxMembers: 2, Location: "Park", StartsAt: now, EndsAt: now.Add(48 * time.Hour)}
	if err := validGroup(&base); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"private", "no_end", "reverse", "no_location"} {
		t.Run(kind, func(t *testing.T) {
			input := base
			switch kind {
			case "private":
				input.Visibility = "private"
			case "no_end":
				input.EndsAt = time.Time{}
			case "reverse":
				input.EndsAt = now.Add(-time.Hour)
			case "no_location":
				input.Location = " "
			}
			if validGroup(&input) == nil {
				t.Fatal("invalid input accepted")
			}
		})
	}
}

// Uses only a unique test schema. Never migrates or truncates application tables.
func TestGroupDatabase(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schema := fmt.Sprintf("group_test_%d", time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Seed the old schema to verify migration preserves old private groups/events.
	for _, file := range []string{"dev_members.sql", "001_features.sql"} {
		sql, e := migrations.ReadFile("sql/" + file)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, string(sql), pgx.QueryExecModeSimpleProtocol); e != nil {
			t.Fatal(e)
		}
	}
	var legacy int64
	err = pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `INSERT INTO groups(owner_id,name,description,sport,visibility,max_members) VALUES (1,'Legacy','','running','private',10) RETURNING id`).Scan(&legacy); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `INSERT INTO group_members(group_id,user_id) VALUES ($1,1)`, legacy)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO group_events(group_id,created_by,title,sport,starts_at,location) VALUES ($1,1,'Old event','running',now(),'Park')`, legacy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `CREATE TABLE fittogether_migrations(name text PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now()); INSERT INTO fittogether_migrations(name) VALUES ('001_features.sql'),('dev_members.sql')`, pgx.QueryExecModeSimpleProtocol)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyMigrations(ctx, db, true); err != nil {
		t.Fatal(err)
	}
	if err = applyMigrations(ctx, db, true); err != nil {
		t.Fatal("repeat migration:", err)
	}
	var preserved bool
	err = db.QueryRow(ctx, `SELECT visibility='private' AND starts_at IS NULL AND EXISTS(SELECT 1 FROM legacy_group_events WHERE group_id=$1) FROM groups WHERE id=$1`, legacy).Scan(&preserved)
	if err != nil || !preserved {
		t.Fatal("legacy data changed", err)
	}
	s := &server{db: db}
	makeGroup := func(visibility string, capacity int, end time.Time) int64 {
		t.Helper()
		var id int64
		e := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
			if e := tx.QueryRow(ctx, `INSERT INTO groups(owner_id,name,description,sport,visibility,max_members,starts_at,ends_at,location) VALUES (1,$1,'','running',$2,$3,$4,$5,'Park') RETURNING id`, fmt.Sprintf("Group-%d", time.Now().UnixNano()), visibility, capacity, end.Add(-48*time.Hour), end).Scan(&id); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `INSERT INTO group_members(group_id,user_id) VALUES ($1,1)`, id)
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	call := func(h handler, id, user int64, body string, params map[string]string) error {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.SetPathValue("group", fmt.Sprint(id))
		for k, v := range params {
			r.SetPathValue(k, v)
		}
		return h(httptest.NewRecorder(), r, user)
	}
	t.Run("create_read_update_and_legacy_setup", func(t *testing.T) {
		now := time.Now().UTC()
		body, _ := json.Marshal(groupInput{Name: "API group", Sport: "running", Visibility: "closed", MaxMembers: 3, Location: "Park", StartsAt: now.Add(time.Hour), EndsAt: now.Add(49 * time.Hour)})
		w := httptest.NewRecorder()
		if e := s.createGroup(w, httptest.NewRequest("POST", "/", strings.NewReader(string(body))), 1); e != nil {
			t.Fatal(e)
		}
		var response struct {
			ID          int64 `json:"id"`
			MemberCount int   `json:"member_count"`
			Accepting   bool  `json:"accepting_members"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
			t.Fatal(e)
		}
		if response.ID == 0 || response.MemberCount != 1 || !response.Accepting {
			t.Fatalf("bad response: %s", w.Body.String())
		}
		if e := call(s.getGroup, response.ID, 2, "", nil); e != nil {
			t.Fatal("public details:", e)
		}
		if e := call(s.groups, 0, 2, "", nil); e != nil {
			t.Fatal("list:", e)
		}
		if e := call(s.updateGroup, response.ID, 2, `{"location":"Other"}`, nil); e == nil {
			t.Fatal("non-owner edited group")
		}
		if e := call(s.updateGroup, response.ID, 1, `{"location":"Other","closed_at":null}`, nil); e != nil {
			t.Fatal("patch:", e)
		}
		if e := call(s.getGroup, legacy, 2, "", nil); e == nil {
			t.Fatal("legacy private group leaked")
		}
		if e := call(s.joinGroup, legacy, 2, "", nil); e == nil {
			t.Fatal("legacy group accepted member before setup")
		}
		setup, _ := json.Marshal(map[string]any{"starts_at": now.Add(time.Hour), "ends_at": now.Add(49 * time.Hour), "location": "Park", "visibility": "closed"})
		if e := call(s.updateGroup, legacy, 1, string(setup), nil); e != nil {
			t.Fatal("legacy setup:", e)
		}
		if e := call(s.getGroup, legacy, 2, "", nil); e != nil {
			t.Fatal("converted group not discoverable:", e)
		}
	})
	t.Run("capacity_and_closed_admissions", func(t *testing.T) {
		id := makeGroup("open", 2, time.Now().Add(time.Hour))
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, user := range []int64{2, 3} {
			wg.Add(1)
			go func(u int64) { defer wg.Done(); results <- call(s.joinGroup, id, u, "", nil) }(user)
		}
		wg.Wait()
		close(results)
		success := 0
		for e := range results {
			if e == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("expected one admission, got %d", success)
		}
		id = makeGroup("closed", 3, time.Now().Add(time.Hour))
		if e := call(s.joinGroup, id, 2, "", nil); e != nil {
			t.Fatal(e)
		}
		if e := call(s.invite, id, 1, `{"user_id":3}`, nil); e != nil {
			t.Fatal(e)
		}
		var request, invite int64
		if e := db.QueryRow(ctx, "SELECT id FROM group_join_requests WHERE group_id=$1", id).Scan(&request); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(ctx, "SELECT id FROM group_invites WHERE group_id=$1", id).Scan(&invite); e != nil {
			t.Fatal(e)
		}
		if _, e := db.Exec(ctx, "UPDATE groups SET closed_at=now() WHERE id=$1", id); e != nil {
			t.Fatal(e)
		}
		if e := call(s.reviewRequest, id, 1, `{"status":"approved"}`, map[string]string{"request": fmt.Sprint(request)}); e == nil {
			t.Fatal("approved after registration closed")
		}
		if e := call(s.reviewInvite, id, 3, `{"status":"accepted"}`, map[string]string{"invite": fmt.Sprint(invite)}); e == nil {
			t.Fatal("accepted after registration closed")
		}
	})
	t.Run("completion_chat_and_idempotency", func(t *testing.T) {
		id := makeGroup("open", 3, time.Now().Add(time.Hour))
		if e := call(s.joinGroup, id, 2, "", nil); e != nil {
			t.Fatal(e)
		}
		if e := call(s.completeGroup, id, 2, "", nil); e == nil {
			t.Fatal("member completed activity")
		}
		if e := call(s.completeGroup, id, 1, "", nil); e != nil {
			t.Fatal(e)
		}
		var first, second time.Time
		if e := db.QueryRow(ctx, "SELECT completed_at FROM groups WHERE id=$1", id).Scan(&first); e != nil {
			t.Fatal(e)
		}
		if e := call(s.completeGroup, id, 1, "", nil); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(ctx, "SELECT completed_at FROM groups WHERE id=$1", id).Scan(&second); e != nil {
			t.Fatal(e)
		}
		if !first.Equal(second) {
			t.Fatal("completion was reset")
		}
		if e := call(s.sendMessage, id, 2, `{"content":"Thanks!"}`, nil); e != nil {
			t.Fatal("chat after completion:", e)
		}
		if e := call(s.joinGroup, id, 3, "", nil); e == nil {
			t.Fatal("joined completed activity")
		}
		if e := call(s.messages, id, 3, "", nil); e == nil {
			t.Fatal("outsider read messages")
		}
	})
	t.Run("worker_cascade_and_cancellation", func(t *testing.T) {
		due := makeGroup("open", 3, time.Now().Add(-25*time.Hour))
		expired := makeGroup("open", 3, time.Now().Add(-49*time.Hour))
		if _, e := db.Exec(ctx, "INSERT INTO group_messages(group_id,sender_id,content) VALUES ($1,1,'old')", expired); e != nil {
			t.Fatal(e)
		}
		if e := s.sweepGroups(ctx); e != nil {
			t.Fatal(e)
		}
		if e := s.sweepGroups(ctx); e != nil {
			t.Fatal("repeat worker:", e)
		}
		var ok bool
		if e := db.QueryRow(ctx, "SELECT status='completed' AND completed_at=ends_at+interval '24 hours' FROM groups WHERE id=$1", due).Scan(&ok); e != nil || !ok {
			t.Fatal("wrong completion time", e)
		}
		if e := db.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM groups WHERE id=$1) AND NOT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1) AND NOT EXISTS(SELECT 1 FROM group_messages WHERE group_id=$1)", expired).Scan(&ok); e != nil || !ok {
			t.Fatal("cascade failed", e)
		}
		cancelled := makeGroup("closed", 3, time.Now().Add(72*time.Hour))
		if e := call(s.disband, cancelled, 1, "", nil); e != nil {
			t.Fatal(e)
		}
		if e := call(s.sendMessage, cancelled, 1, `{"content":"test"}`, nil); e == nil {
			t.Fatal("cancelled chat allowed")
		}
		if _, e := db.Exec(ctx, "UPDATE groups SET cancelled_at=now()-interval '25 hours' WHERE id=$1", cancelled); e != nil {
			t.Fatal(e)
		}
		if e := s.sweepGroups(ctx); e != nil {
			t.Fatal(e)
		}
		if e := db.QueryRow(ctx, "SELECT NOT EXISTS(SELECT 1 FROM groups WHERE id=$1)", cancelled).Scan(&ok); e != nil || !ok {
			t.Fatal("cancel cleanup failed", e)
		}
	})
}
