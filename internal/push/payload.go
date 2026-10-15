package push

import (
	"encoding/json"
	"strconv"

	"github.com/google/uuid"
)

// Kinds of push.
const (
	KindAlert = "alert"
	KindBadge = "badge"
)

// Push is everything a push says, and the whole body of a relay send: identifiers, the
// notification type, the ticket number and the badge. It has no field for ticket text,
// names or addresses, so none can reach the relay.
type Push struct {
	Kind           string     `json:"kind" enum:"alert,badge"`
	Type           string     `json:"type,omitempty" maxLength:"64" pattern:"^[a-z][a-z0-9_]*$" doc:"Notification type (alert)"`
	NotificationID *uuid.UUID `json:"notification_id,omitempty"`
	TicketID       *uuid.UUID `json:"ticket_id,omitempty"`
	TicketNumber   *int64     `json:"ticket_number,omitempty" minimum:"0"`
	WorkspaceID    *uuid.UUID `json:"workspace_id,omitempty"`
	Badge          int        `json:"badge" minimum:"0" maximum:"1000000"`
}

// Priority is the apns-priority: 10 for alerts, 5 for badge updates.
func (p Push) Priority() int {
	if p.Kind == KindBadge {
		return 5
	}
	return 10
}

// CollapseID makes a repeated push of the same notification replace the earlier one.
func (p Push) CollapseID() string {
	if p.Kind == KindBadge {
		return "badge"
	}
	if p.NotificationID != nil {
		return p.NotificationID.String()
	}
	return ""
}

// maxSubtitle bounds the subtitle so the payload stays far below APNs' 4 KB.
const maxSubtitle = 120

// Payload builds the APNs JSON for p. server is the instance's API_URL; subtitle (the
// ticket subject) is set only by an instance sending directly, never by the relay.
func Payload(p Push, server, subtitle string) ([]byte, error) {
	if p.Kind == KindBadge {
		return json.Marshal(map[string]any{"aps": map[string]any{"badge": p.Badge}})
	}
	alert := map[string]any{"title-loc-key": "push." + p.Type + ".title", "loc-key": "push." + p.Type + ".body"}
	if p.TicketNumber != nil {
		alert["loc-args"] = []string{strconv.FormatInt(*p.TicketNumber, 10)}
	}
	if subtitle != "" {
		if r := []rune(subtitle); len(r) > maxSubtitle {
			subtitle = string(r[:maxSubtitle-1]) + "…"
		}
		alert["subtitle"] = subtitle
	}
	aps := map[string]any{"alert": alert, "badge": p.Badge, "sound": "default"}
	if p.TicketID != nil {
		aps["thread-id"] = p.TicketID.String()
		aps["category"] = "TICKET"
	} else {
		aps["thread-id"] = p.Type
	}
	od := map[string]any{"v": 1, "server": server, "type": p.Type}
	if p.WorkspaceID != nil {
		od["workspace_id"] = p.WorkspaceID.String()
	}
	if p.NotificationID != nil {
		od["notification_id"] = p.NotificationID.String()
	}
	if p.TicketID != nil {
		od["ticket_id"] = p.TicketID.String()
	}
	if p.TicketNumber != nil {
		od["ticket_number"] = *p.TicketNumber
	}
	return json.Marshal(map[string]any{"aps": aps, "od": od})
}
