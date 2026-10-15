package core

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/auth"
	"github.com/useopendesk/opendesk/internal/crypto"
	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/displayname"
	"github.com/useopendesk/opendesk/internal/httpx"
	"github.com/useopendesk/opendesk/internal/ids"
	"github.com/useopendesk/opendesk/internal/push"
)

const (
	devicesPerUserHour = 60
	maxMutedTypes      = 32
)

var notificationTypeRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// CoreDevice is a phone that receives pushes (contracts/native.md "Devices").
type CoreDevice struct {
	ID          uuid.UUID  `json:"id"`
	Platform    string     `json:"platform" enum:"ios"`
	Name        string     `json:"name"`
	AppVersion  string     `json:"app_version"`
	Environment string     `json:"environment" enum:"production,sandbox"`
	MutedTypes  []string   `json:"muted_types" doc:"Notification types this device is not pushed"`
	Current     bool       `json:"current" doc:"Bound to the calling session"`
	CreatedAt   time.Time  `json:"created_at"`
	LastPushAt  *time.Time `json:"last_push_at"`
}

const deviceCols = `d.id, d.platform, d.name, d.app_version, d.environment, d.muted_types, d.session_id, d.created_at, d.last_push_at`

func scanDevice(row pgx.Row, current uuid.UUID) (CoreDevice, error) {
	var x CoreDevice
	var sid uuid.UUID
	err := row.Scan(&x.ID, &x.Platform, &x.Name, &x.AppVersion, &x.Environment, &x.MutedTypes, &sid, &x.CreatedAt, &x.LastPushAt)
	if x.MutedTypes == nil {
		x.MutedTypes = []string{}
	}
	x.Current = sid == current
	return x, err
}

// liveDevice joins a device to its session: a device counts only while its session lives.
const liveDevice = `push_devices d JOIN sessions s ON s.id = d.session_id AND s.revoked_at IS NULL AND s.expires_at > now()`

func loadDevice(ctx context.Context, q db.Querier, p *auth.Principal, id uuid.UUID) (CoreDevice, error) {
	return scanDevice(q.QueryRow(ctx, `SELECT `+deviceCols+` FROM `+liveDevice+` WHERE d.id = $1 AND d.user_id = $2`, id, p.UserID), p.SessionID)
}

type coreDeviceCreate struct {
	Platform    string `json:"platform" enum:"ios"`
	Token       string `json:"token,omitempty" maxLength:"200" doc:"APNs device token, hex (push = direct)"`
	RelayToken  string `json:"relay_token,omitempty" maxLength:"2048" doc:"From the push relay (push = relay)"`
	Environment string `json:"environment" enum:"production,sandbox"`
	AppVersion  string `json:"app_version" maxLength:"40"`
	Name        string `json:"name" maxLength:"200"`
}

func (s *svc) registerDevices(api huma.API) {
	tags := []string{"Account"}

	register(s, api, huma.Operation{OperationID: "create-device", Method: http.MethodPost, Path: "/v1/me/devices",
		Summary: "Register this phone for pushes", Tags: tags},
		func(ctx context.Context, in *struct{ Body coreDeviceCreate }) (*struct{ Body CoreDevice }, error) {
			p, err := auth.RequireUser(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			if err := s.limit("devices:"+p.UserID.String(), devicesPerUserHour, time.Hour); err != nil {
				return nil, err
			}
			b := in.Body
			var mode, secret string
			switch {
			case b.Token != "" && b.RelayToken == "":
				tok, ok := push.NormalizeDeviceToken(b.Token)
				if !ok {
					return nil, httpx.Validation("token must be an APNs device token in hex.")
				}
				mode, secret = "direct", tok
			case b.RelayToken != "" && b.Token == "":
				mode, secret = "relay", b.RelayToken
			default:
				return nil, httpx.Validation("Send exactly one of token and relay_token.")
			}
			if s.d.Cfg.PushMode() != mode {
				return nil, httpx.E(422, "push_unavailable", "This server does not send notifications that way.")
			}
			sealed, err := s.d.Box.Seal([]byte(secret))
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			name := displayname.CleanOr(b.Name, displayname.Workspace, "iPhone")
			version := truncate(b.AppVersion, 40)
			hash := crypto.HashToken(mode + ":" + secret)
			var dev CoreDevice
			err = s.tx(ctx, func(tx pgx.Tx) error {
				var live bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sessions WHERE id = $1 AND user_id = $2
					AND revoked_at IS NULL AND expires_at > now())`, p.SessionID, p.UserID).Scan(&live); err != nil {
					return err
				}
				if !live {
					return httpx.E(401, "unauthenticated", auth.ErrUnauthenticated.Error())
				}
				var id, prevUser, prevSession uuid.UUID
				err := tx.QueryRow(ctx, `SELECT id, user_id, session_id FROM push_devices WHERE token_hash = $1 FOR UPDATE`, hash).
					Scan(&id, &prevUser, &prevSession)
				switch {
				case db.IsNotFound(err):
					id = ids.New()
					if _, err := tx.Exec(ctx, `INSERT INTO push_devices (id, user_id, session_id, platform, mode, token_hash, token_sealed,
						environment, name, app_version) VALUES ($1, $2, $3, 'ios', $4, $5, $6, $7, $8, $9)`,
						id, p.UserID, p.SessionID, mode, hash, sealed, b.Environment, name, version); err != nil {
						return err
					}
				case err != nil:
					return err
				default:
					// The token now belongs to this sign-in. Another person's mute choices do
					// not carry over.
					if _, err := tx.Exec(ctx, `UPDATE push_devices SET user_id = $2, session_id = $3, token_sealed = $4, environment = $5,
						name = $6, app_version = $7, updated_at = now(),
						muted_types = CASE WHEN user_id = $2 THEN muted_types ELSE '{}' END,
						last_push_at = CASE WHEN user_id = $2 THEN last_push_at ELSE NULL END
						WHERE id = $1`, id, p.UserID, p.SessionID, sealed, b.Environment, name, version); err != nil {
						return err
					}
				}
				// One phone per session: a token the app replaced stops receiving pushes.
				if _, err := tx.Exec(ctx, `DELETE FROM push_devices WHERE session_id = $1 AND id <> $2`, p.SessionID, id); err != nil {
					return err
				}
				if prevUser != p.UserID || prevSession != p.SessionID {
					if _, err := auditUser(ctx, tx, p, p.UserID, "push_device.registered", map[string]any{"device_id": id,
						"platform": "ios", "environment": b.Environment, "name": name}); err != nil {
						return err
					}
				}
				dev, err = loadDevice(ctx, tx, p, id)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body CoreDevice }{dev}, nil
		})

	register(s, api, huma.Operation{OperationID: "list-devices", Method: http.MethodGet, Path: "/v1/me/devices",
		Summary: "List your phones that receive pushes", Tags: tags},
		func(ctx context.Context, _ *struct{}) (*struct{ Body []CoreDevice }, error) {
			p, err := auth.RequireUser(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			rows, err := s.d.DB.Pool.Query(ctx, `SELECT `+deviceCols+` FROM `+liveDevice+` WHERE d.user_id = $1
				ORDER BY d.created_at DESC, d.id DESC LIMIT 100`, p.UserID)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (CoreDevice, error) { return scanDevice(r, p.SessionID) })
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body []CoreDevice }{list}, nil
		})

	register(s, api, huma.Operation{OperationID: "update-device", Method: http.MethodPatch, Path: "/v1/me/devices/{id}",
		Summary: "Choose which notifications a phone is pushed", Tags: tags},
		func(ctx context.Context, in *struct {
			ID   string `path:"id"`
			Body struct {
				MutedTypes *[]string `json:"muted_types,omitempty" maxItems:"32"`
			}
		}) (*struct{ Body CoreDevice }, error) {
			p, err := auth.RequireUser(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := parseID(in.ID, "Device")
			if err != nil {
				return nil, err
			}
			var muted []string
			if in.Body.MutedTypes != nil {
				muted = []string{}
				for _, t := range *in.Body.MutedTypes {
					if !notificationTypeRe.MatchString(t) {
						return nil, httpx.Validation("muted_types holds notification types such as customer_replied.")
					}
					if !slices.Contains(muted, t) {
						muted = append(muted, t)
					}
				}
				if len(muted) > maxMutedTypes {
					return nil, httpx.Validation("Too many muted types.")
				}
				slices.Sort(muted)
			}
			var dev CoreDevice
			err = s.tx(ctx, func(tx pgx.Tx) error {
				if _, err := loadDevice(ctx, tx, p, id); err != nil {
					if db.IsNotFound(err) {
						return httpx.NotFound("Device")
					}
					return err
				}
				if muted != nil {
					if _, err := tx.Exec(ctx, `UPDATE push_devices SET muted_types = $3, updated_at = now() WHERE id = $1 AND user_id = $2`,
						id, p.UserID, muted); err != nil {
						return err
					}
				}
				dev, err = loadDevice(ctx, tx, p, id)
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body CoreDevice }{dev}, nil
		})

	register(s, api, huma.Operation{OperationID: "delete-device", Method: http.MethodDelete, Path: "/v1/me/devices/{id}",
		Summary: "Stop pushes to a phone", Tags: tags, DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID string `path:"id"`
		}) (*struct{}, error) {
			p, err := auth.RequireUser(ctx)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			id, err := parseID(in.ID, "Device")
			if err != nil {
				return nil, err
			}
			err = s.tx(ctx, func(tx pgx.Tx) error {
				tag, err := tx.Exec(ctx, `DELETE FROM push_devices WHERE id = $1 AND user_id = $2`, id, p.UserID)
				if err != nil {
					return err
				}
				if tag.RowsAffected() == 0 {
					return httpx.NotFound("Device")
				}
				_, err = auditUser(ctx, tx, p, p.UserID, "push_device.removed", map[string]any{"device_id": id})
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return nil, nil
		})
}
