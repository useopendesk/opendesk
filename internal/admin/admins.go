package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/useopendesk/opendesk/internal/db"
	"github.com/useopendesk/opendesk/internal/httpx"
)

type InstanceAdminUser struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

type InstanceAdminGrantor struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

type InstanceAdmin struct {
	User        InstanceAdminUser     `json:"user"`
	Role        Role                  `json:"role" enum:"viewer,support,super"`
	GrantedBy   *InstanceAdminGrantor `json:"granted_by"`
	TOTPEnabled bool                  `json:"totp_enabled"`
	CreatedAt   time.Time             `json:"created_at"`
}

const adminCols = `u.id, u.email, u.name, a.role, g.id, g.email, u.totp_enabled, a.created_at`

func scanAdmin(r pgx.Row) (InstanceAdmin, error) {
	var x InstanceAdmin
	var gid *uuid.UUID
	var gemail *string
	err := r.Scan(&x.User.ID, &x.User.Email, &x.User.Name, &x.Role, &gid, &gemail, &x.TOTPEnabled, &x.CreatedAt)
	if gid != nil && gemail != nil {
		x.GrantedBy = &InstanceAdminGrantor{ID: *gid, Email: *gemail}
	}
	return x, err
}

// ListAdmins lists the instance admins, supers first.
func ListAdmins(ctx context.Context, q db.Querier) ([]InstanceAdmin, error) {
	rows, err := q.Query(ctx, `SELECT `+adminCols+` FROM instance_admins a JOIN users u ON u.id = a.user_id
		LEFT JOIN users g ON g.id = a.granted_by
		ORDER BY CASE a.role WHEN 'super' THEN 0 WHEN 'support' THEN 1 ELSE 2 END, a.created_at`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (InstanceAdmin, error) { return scanAdmin(r) })
	if list == nil {
		list = []InstanceAdmin{}
	}
	return list, err
}

func loadAdminRow(ctx context.Context, q db.Querier, uid uuid.UUID) (InstanceAdmin, error) {
	x, err := scanAdmin(q.QueryRow(ctx, `SELECT `+adminCols+` FROM instance_admins a JOIN users u ON u.id = a.user_id
		LEFT JOIN users g ON g.id = a.granted_by WHERE a.user_id = $1`, uid))
	if db.IsNotFound(err) {
		return x, httpx.NotFound("Admin")
	}
	return x, err
}

// Actor is who changes the admins: a signed-in admin, or the command line.
type Actor struct {
	Type   string // admin | cli
	UserID *uuid.UUID
	Email  string
	IP     string
}

// CLIActor is the command line on the machine that runs the instance.
var CLIActor = Actor{Type: "cli", Email: "cli"}

func (a *Admin) actor() Actor {
	return Actor{Type: "admin", UserID: &a.UserID, Email: a.Email, IP: a.IP}
}

// Grant makes the user with this email an admin with role, or changes their role. It
// reports whether they were an admin already.
func Grant(ctx context.Context, tx pgx.Tx, email string, role Role, by Actor) (InstanceAdmin, bool, error) {
	if !role.Valid() {
		return InstanceAdmin{}, false, httpx.Validation("role must be viewer, support or super")
	}
	email = normEmail(email)
	var uid uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&uid)
	if email == "" || db.IsNotFound(err) {
		return InstanceAdmin{}, false, httpx.E(404, "user_not_found", "No account uses this email address. The person signs up first.")
	}
	if err != nil {
		return InstanceAdmin{}, false, err
	}
	var prev *Role
	if err := tx.QueryRow(ctx, `SELECT role FROM instance_admins WHERE user_id = $1 FOR UPDATE`, uid).Scan(&prev); err != nil && !db.IsNotFound(err) {
		return InstanceAdmin{}, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO instance_admins (user_id, role, granted_by) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET role = EXCLUDED.role, updated_at = now()`, uid, role, by.UserID); err != nil {
		return InstanceAdmin{}, false, err
	}
	action, data := "admin.granted", map[string]any{"role": role}
	if prev != nil {
		action, data = "admin.role_changed", map[string]any{"from": *prev, "to": role}
	}
	if err := writeAudit(ctx, tx, by.Type, by.UserID, by.Email, action, "admin", uid.String(), data, by.IP); err != nil {
		return InstanceAdmin{}, false, err
	}
	x, err := loadAdminRow(ctx, tx, uid)
	return x, prev != nil, err
}

// Revoke removes an admin and ends their admin sessions.
func Revoke(ctx context.Context, tx pgx.Tx, uid uuid.UUID, by Actor) error {
	var role Role
	err := tx.QueryRow(ctx, `DELETE FROM instance_admins WHERE user_id = $1 RETURNING role`, uid).Scan(&role)
	if db.IsNotFound(err) {
		return httpx.NotFound("Admin")
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, uid); err != nil {
		return err
	}
	return writeAudit(ctx, tx, by.Type, by.UserID, by.Email, "admin.revoked", "admin", uid.String(), map[string]any{"role": role}, by.IP)
}

// UserIDByEmail finds an account.
func UserIDByEmail(ctx context.Context, q db.Querier, email string) (uuid.UUID, error) {
	var uid uuid.UUID
	err := q.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, normEmail(email)).Scan(&uid)
	if db.IsNotFound(err) {
		return uid, httpx.E(404, "user_not_found", "No account uses this email address.")
	}
	return uid, err
}

// supers counts the super admins.
func supers(ctx context.Context, q db.Querier) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM instance_admins WHERE role = 'super'`).Scan(&n)
	return n, err
}

func (s *svc) registerAdmins(api huma.API) {
	op(s, api, huma.Operation{OperationID: "admin-list-admins", Method: http.MethodGet, Path: "/v1/admin/admins", Summary: "List instance admins"},
		func(ctx context.Context, _ *struct{}) (*struct{ Body []InstanceAdmin }, error) {
			if _, err := require(ctx, PermRead); err != nil {
				return nil, err
			}
			list, err := ListAdmins(ctx, s.d.DB.Pool)
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body []InstanceAdmin }{list}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-grant-admin", Method: http.MethodPost, Path: "/v1/admin/admins", Summary: "Make someone an instance admin"},
		func(ctx context.Context, in *struct {
			Body struct {
				Code  string `json:"code,omitempty" maxLength:"64" doc:"A current authenticator code (or a recovery code) of the acting admin"`
				Email string `json:"email" maxLength:"320"`
				Role  Role   `json:"role" enum:"viewer,support,super"`
			}
		}) (*struct{ Body InstanceAdmin }, error) {
			a, err := require(ctx, PermAdminsManage)
			if err != nil {
				return nil, err
			}
			var x InstanceAdmin
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := s.stepUp(ctx, tx, a, in.Body.Code); err != nil {
					return err
				}
				var totp, suspended, isAdmin bool
				err := tx.QueryRow(ctx, `SELECT u.totp_enabled, u.suspended_at IS NOT NULL, EXISTS (SELECT 1 FROM instance_admins WHERE user_id = u.id)
					FROM users u WHERE u.email = $1`, normEmail(in.Body.Email)).Scan(&totp, &suspended, &isAdmin)
				switch {
				case db.IsNotFound(err):
					return httpx.E(404, "user_not_found", "No account uses this email address. The person signs up first.")
				case err != nil:
					return err
				case isAdmin:
					return httpx.E(409, "already_admin", "This person is an admin already. Change their role instead.")
				case suspended:
					return httpx.E(409, "account_suspended", "This account is suspended. Restore it first.")
				case !totp:
					return httpx.E(409, "admin_mfa_setup_required", "This person turns on two-factor authentication in the app first.")
				}
				x, _, err = Grant(ctx, tx, in.Body.Email, in.Body.Role, a.actor())
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body InstanceAdmin }{x}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-update-admin", Method: http.MethodPatch, Path: "/v1/admin/admins/{user_id}", Summary: "Change an admin's role"},
		func(ctx context.Context, in *struct {
			UserID string `path:"user_id"`
			Body   struct {
				Code string `json:"code,omitempty" maxLength:"64" doc:"A current authenticator code (or a recovery code) of the acting admin"`
				Role Role   `json:"role" enum:"viewer,support,super"`
			}
		}) (*struct{ Body InstanceAdmin }, error) {
			a, err := require(ctx, PermAdminsManage)
			if err != nil {
				return nil, err
			}
			uid, err := parseID(in.UserID, "Admin")
			if err != nil {
				return nil, err
			}
			var x InstanceAdmin
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := s.stepUp(ctx, tx, a, in.Body.Code); err != nil {
					return err
				}
				if uid == a.UserID {
					return httpx.E(409, "conflict", "You cannot change your own role.")
				}
				cur, err := loadAdminRow(ctx, tx, uid)
				if err != nil {
					return err
				}
				if cur.Role == RoleSuper && in.Body.Role != RoleSuper {
					if n, err := supers(ctx, tx); err != nil || n <= 1 {
						if err != nil {
							return err
						}
						return httpx.E(409, "last_super", "Keep at least one super admin.")
					}
				}
				x, _, err = Grant(ctx, tx, cur.User.Email, in.Body.Role, a.actor())
				return err
			})
			if err != nil {
				return nil, httpx.Wrap(err)
			}
			return &struct{ Body InstanceAdmin }{x}, nil
		})

	op(s, api, huma.Operation{OperationID: "admin-revoke-admin", Method: http.MethodDelete, Path: "/v1/admin/admins/{user_id}",
		Summary: "Remove an instance admin", DefaultStatus: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			UserID string `path:"user_id"`
			Body   codeBody
		}) (*struct{}, error) {
			a, err := require(ctx, PermAdminsManage)
			if err != nil {
				return nil, err
			}
			uid, err := parseID(in.UserID, "Admin")
			if err != nil {
				return nil, err
			}
			err = s.d.DB.Tx(ctx, func(tx pgx.Tx) error {
				if err := s.stepUp(ctx, tx, a, in.Body.Code); err != nil {
					return err
				}
				if uid == a.UserID {
					return httpx.E(409, "conflict", "You cannot remove yourself. Another super admin can.")
				}
				cur, err := loadAdminRow(ctx, tx, uid)
				if err != nil {
					return err
				}
				if cur.Role == RoleSuper {
					if n, err := supers(ctx, tx); err != nil || n <= 1 {
						if err != nil {
							return err
						}
						return httpx.E(409, "last_super", "Keep at least one super admin.")
					}
				}
				return Revoke(ctx, tx, uid, a.actor())
			})
			return nil, httpx.Wrap(err)
		})
}
