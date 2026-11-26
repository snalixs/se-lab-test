# Group activity API

One group is one activity. Membership is attendance, including the owner.
There is no separate event/RSVP workflow and no warmup-set integration yet.

## Apply the schema

Run `go run . -migrate` from `backend`, then restart the API. Migration 002 is
transactional and is applied once. It does not modify post tables.

Existing groups retain their original data. Missing schedules are NOT invented.
Such groups return `needs_setup: true`; their owner must PATCH `starts_at`,
`ends_at`, and `location` before admissions/completion. Old private groups remain
visible only to existing members until their owner explicitly changes visibility
to `open` or `closed` in the same PATCH. This makes that group discoverable.
Existing disbanded groups have no known cancellation timestamp; they remain
retained for a separate explicit data migration, not automatically purged.

The new schedule/lifecycle/visibility CHECK constraints use NOT VALID to preserve
legacy rows while enforcing the contract on every new/updated row. Once legacy
data is resolved, the constraints can be validated. Old `group_events` and
`event_rsvps` tables are renamed to `legacy_group_events` and
`legacy_event_rsvps`; their data remains attached to the original group and is
removed with that group when it is eventually deleted. Do not run the standalone
`handoff/sql/*.sql` files on an already migrated database.

## Create and update

POST `/api/groups`:

```json
{
  "name": "Weekend Running",
  "description": "Two-day running activity",
  "sport": "running",
  "visibility": "closed",
  "max_members": 20,
  "starts_at": "2026-12-12T08:00:00+07:00",
  "ends_at": "2026-12-13T18:00:00+07:00",
  "closed_at": "2026-12-11T20:00:00+07:00",
  "location": "SUT Stadium",
  "cover_url": null
}
```

Times use RFC3339 with an offset; PostgreSQL stores `timestamptz`. New activities
must start in the future; ends_at must be after starts_at. Closed_at can be null
(no independent registration deadline). A finished/cancelled activity never
accepts admissions even without closed_at. Max_members includes the owner.

PATCH `/api/groups/{group}` accepts the same editable fields, all optional.
`closed_at: null` clears the deadline; omitting it preserves it. `cover_url` works
the same way. Server-owned status, owner_id, completed_at, cancelled_at, and
updated_at cannot be set through this endpoint. Time fields cannot be used to
revive an activity whose automatic completion deadline already passed.
Use the dedicated owner-transfer endpoint to transfer ownership.

GET responses include all activity times, status, member_count, is_member,
needs_setup, accepting_members, and delete_at (null before completion/cancellation).
Public discovery lists active open/closed groups. `scope=mine` also includes
finished/cancelled groups retained for the current member. Messages and member
lists require membership. Schedule/detail changes notify other members.

## Admissions

- Open: POST `/api/groups/{group}/join` adds the caller immediately.
- Closed: the same endpoint creates a pending request. The owner approves/rejects
  via PUT `/api/groups/{group}/join-requests/{request}`.
- Closed: the owner may POST `/api/groups/{group}/invites` with user_id; the
  recipient answers via PUT `/api/groups/{group}/invites/{invite}` with status
  accepted/declined. Acceptance adds membership without another approval.
- Pending requests/invites do not reserve seats. Capacity is checked when adding
  membership under a group row lock, including concurrent approvals/acceptances.
- Closing registration blocks new requests, invites, approvals and acceptances.
  Existing pending records retain their historical status; they cannot admit a
  member while closed. Reopening registration can make them actionable again.
- Owners must transfer ownership before leaving. Removing a member cancels that
  user's attendance; counts come from group_members, never invitation statuses.

## Completion and retention

POST `/api/groups/{group}/complete` is owner-only and available after starts_at.
It sets status=completed and completed_at to the current time. Calling it again
does not extend retention. Before start, the owner should cancel instead.

Without manual completion, the deadline is ends_at + 24 hours. The worker uses
that deadline as completed_at even if it runs late. All admissions and mutable
activity settings are blocked after completion. Existing members CAN continue
chatting until deletion. They cannot change membership after completion.

DELETE `/api/groups/{group}` cancels an active activity: status=disbanded,
cancelled_at=current time. Repeating it does not extend retention. Cancelled
groups are readable to members, but cannot accept new messages or members.

Deletion is due 24 hours after completed_at or cancelled_at. The API stops serving
the group at that deadline even if cleanup has not run yet. A worker runs at API
startup and every minute in batches of 200. Physical deletion may lag by a polling
interval, backlog, or server downtime; this is not a hard wall-clock guarantee.
The worker catches up after restart, uses row locks to avoid duplicate completion,
and cascades group-owned rows only. Users/profiles and posts are not deleted.
updated_at is maintained by a database trigger.

Old `/events` and `/events/{event}/rsvp` routes return 410 Gone. The current
Frontend must be updated to send required activity fields, remove its nested
event/RSVP controls, and add the completion action. This backend change does not
silently translate old UI requests into made-up schedules.

## Tests

`go test ./...` runs unit tests. To also exercise migrations, concurrency,
permissions, completion, chat and cascading deletes on PostgreSQL:

```powershell
$env:TEST_DATABASE_URL = $env:DATABASE_URL # set to your local test DB connection
go test -v ./...
```

Integration tests create and remove a uniquely named `group_test_*` schema. They
do not migrate or truncate application tables. The test account must be allowed
to create schemas. TEST_DATABASE_URL is deliberately not loaded from .env.
