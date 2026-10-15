package hd

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/events"
	"github.com/useopendesk/opendesk/internal/httpx"
)

// Team roles (contracts/teams.md). Leads manage the team; observers see its tickets but
// are never auto-assigned.
const (
	TeamLead     = "lead"
	TeamMember   = "member"
	TeamObserver = "observer"
)

// TeamRoles are the roles a team member may hold.
var TeamRoles = []string{TeamLead, TeamMember, TeamObserver}

// TeamBusinessHours is a team's own business hours (contracts/teams.md "Settings scoped to
// a team"). Timezone "" follows the workspace's timezone.
type TeamBusinessHours struct {
	Enabled  bool                   `json:"enabled"`
	Timezone string                 `json:"timezone" required:"false" doc:"IANA time zone; empty follows the workspace"`
	Schedule map[string][]TimeRange `json:"schedule" doc:"mon..sun, up to 3 ranges each"`
	Holidays []Holiday              `json:"holidays" required:"false" maxItems:"100"`
}

// ValidateTeamHours normalises and checks a team's business hours.
func ValidateTeamHours(b TeamBusinessHours) (TeamBusinessHours, error) {
	tz := strings.TrimSpace(b.Timezone)
	if tz != "" {
		if _, err := time.LoadLocation(tz); err != nil || tz == "Local" {
			return b, httpx.Validation("business_hours.timezone must be an IANA time zone like Europe/Berlin, or empty for the workspace's.")
		}
	}
	v, err := ValidateBusinessHours(BusinessHours{Enabled: b.Enabled, Schedule: b.Schedule, Holidays: b.Holidays})
	if err != nil {
		return b, err
	}
	return TeamBusinessHours{Enabled: v.Enabled, Timezone: tz, Schedule: v.Schedule, Holidays: v.Holidays}, nil
}

// ParseTeamHours reads a stored teams.business_hours document; nil when unset.
func ParseTeamHours(raw []byte) *TeamBusinessHours {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var b TeamBusinessHours
	if json.Unmarshal(raw, &b) != nil {
		return nil
	}
	if b.Schedule == nil {
		b.Schedule = map[string][]TimeRange{}
	}
	for _, d := range Weekdays {
		if b.Schedule[d] == nil {
			b.Schedule[d] = []TimeRange{}
		}
	}
	if b.Holidays == nil {
		b.Holidays = []Holiday{}
	}
	return &b
}

// HoursFor returns the business hours and location that apply to a ticket of team: the
// team's own hours when it has them, else the workspace's. teamID may be nil.
func HoursFor(ctx context.Context, q db.Querier, wid uuid.UUID, teamID *uuid.UUID) (BusinessHours, *time.Location, error) {
	s, err := LoadSettings(ctx, q, wid)
	if err != nil {
		return BusinessHours{}, nil, err
	}
	bh, loc := s.Hours(), s.Location()
	if teamID == nil {
		return bh, loc, nil
	}
	var raw []byte
	err = q.QueryRow(ctx, `SELECT business_hours FROM teams WHERE workspace_id = $1 AND id = $2`, wid, *teamID).Scan(&raw)
	if db.IsNotFound(err) {
		return bh, loc, nil
	}
	if err != nil {
		return bh, loc, err
	}
	th := ParseTeamHours(raw)
	if th == nil {
		return bh, loc, nil
	}
	if th.Timezone != "" {
		if l, err := time.LoadLocation(th.Timezone); err == nil {
			loc = l
		}
	}
	return BusinessHours{Enabled: th.Enabled, Schedule: th.Schedule, Holidays: th.Holidays}, loc, nil
}

// TeamPrivate reports whether the workspace's team is private (false when it is gone).
func TeamPrivate(ctx context.Context, q db.Querier, wid uuid.UUID, teamID *uuid.UUID) (bool, error) {
	if teamID == nil {
		return false, nil
	}
	var private bool
	err := q.QueryRow(ctx, `SELECT private FROM teams WHERE workspace_id = $1 AND id = $2`, wid, *teamID).Scan(&private)
	if db.IsNotFound(err) {
		return false, nil
	}
	return private, err
}

// TeamRole returns uid's role on the workspace's team, or "" when not on it.
func TeamRole(ctx context.Context, q db.Querier, wid, teamID, uid uuid.UUID) (string, error) {
	var role string
	err := q.QueryRow(ctx, `SELECT tm.role FROM team_members tm JOIN teams t ON t.id = tm.team_id
		WHERE t.workspace_id = $1 AND tm.team_id = $2 AND tm.user_id = $3`, wid, teamID, uid).Scan(&role)
	if db.IsNotFound(err) {
		return "", nil
	}
	return role, err
}

// ErrNotOnTeam answers an assignee who is not on the private team of the ticket.
func ErrNotOnTeam() error {
	return httpx.E(422, "not_on_team", "The assignee must be on the ticket's private team.")
}

// CheckAssigneeOnTeam refuses an assignee who could not see a ticket of team: the team is
// private and they are not on it (contracts/teams.md "Visibility").
func CheckAssigneeOnTeam(ctx context.Context, q db.Querier, wid uuid.UUID, teamID, assignee *uuid.UUID) error {
	if teamID == nil || assignee == nil {
		return nil
	}
	private, err := TeamPrivate(ctx, q, wid, teamID)
	if err != nil || !private {
		return err
	}
	role, err := TeamRole(ctx, q, wid, *teamID, *assignee)
	if err != nil {
		return err
	}
	if role == "" {
		return ErrNotOnTeam()
	}
	return nil
}

// HiddenEventCap bounds the per-ticket ticket.hidden events one change publishes. A larger
// change publishes team.updated only and clients refetch (contracts/teams.md).
const HiddenEventCap = 100

// PublishTicketHidden tells clients outside teamID to drop the ticket. It goes to the
// workspace topic and carries nothing but the ids.
func PublishTicketHidden(ctx context.Context, tx pgx.Tx, wid, ticketID, teamID uuid.UUID) error {
	_, err := events.Publish(ctx, tx, events.Event{WorkspaceID: wid, Type: "ticket.hidden", Workspace: true,
		Data: map[string]any{"ticket_id": ticketID, "team_id": teamID}})
	return err
}

// Nullable is a body field that tells "absent" from "null": Set is true when the key was
// sent, Null when it was sent as null.
type Nullable[T any] struct {
	Set   bool
	Null  bool
	Value T
}

func (o *Nullable[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}

func (o Nullable[T]) MarshalJSON() ([]byte, error) {
	if !o.Set || o.Null {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// Schema is the schema of T, nullable.
func (o Nullable[T]) Schema(r huma.Registry) *huma.Schema {
	s := *r.Schema(reflect.TypeOf((*T)(nil)).Elem(), false, "")
	s.Nullable = true
	return &s
}

// Ptr returns the value, or nil for null or absent.
func (o Nullable[T]) Ptr() *T {
	if !o.Set || o.Null {
		return nil
	}
	v := o.Value
	return &v
}
