package user

import (
	"context"
	"encoding/json"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/jmoiron/sqlx"
)

// Repository
// Repository interface defines methods for user data access
type Repository interface {
	CreateUser(ctx context.Context, user *User) error
	CreateAdminIfNoAdmin(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
}

// PgRepository implements Repository using PostgreSQL
type PgRepository struct {
	db  *sqlx.DB
	cfg activitypub.Config
}

// NewRepository creates a new instance of PgRepository
func NewRepository(db *sqlx.DB, cfg activitypub.Config) Repository {
	return &PgRepository{
		db:  db,
		cfg: cfg,
	}
}

// Add new user
func (r *PgRepository) CreateUser(ctx context.Context, user *User) error {
	return r.createUserInTx(ctx, user, nil)
}

func (r *PgRepository) CreateAdminIfNoAdmin(ctx context.Context, user *User) error {
	return r.createUserInTx(ctx, user, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('admin-bootstrap'))`); err != nil {
			return err
		}

		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM users WHERE role = $1)`, RoleAdmin); err != nil {
			return err
		}
		if exists {
			return ErrAdminAlreadyExists
		}
		return nil
	})
}

func (r *PgRepository) createUserInTx(ctx context.Context, user *User, beforeInsert func(*sqlx.Tx) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if beforeInsert != nil {
		if err := beforeInsert(tx); err != nil {
			return err
		}
	}
	if err := r.insertUserGraph(ctx, tx, user); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *PgRepository) insertUserGraph(ctx context.Context, tx *sqlx.Tx, user *User) error {
	actorQuery := `
		INSERT INTO actors (
			id, ap_id, type, preferred_username, handle, name, summary,
			inbox_url, outbox_url, followers_url, following_url
		)
		VALUES (
			:id, :ap_id, 'Person', :username, :handle, :name, :summary,
			:inbox_url, :outbox_url, :followers_url, :following_url
		)
	`
	actorParams := map[string]any{
		"id":            user.ID,
		"ap_id":         user.APID,
		"username":      user.Username,
		"handle":        user.Handle,
		"name":          user.Name,
		"summary":       user.Summary,
		"inbox_url":     activitypub.Inbox(user.APID),
		"outbox_url":    activitypub.Outbox(user.APID),
		"followers_url": activitypub.Followers(user.APID),
		"following_url": activitypub.Following(user.APID),
	}
	if _, err := tx.NamedExecContext(ctx, actorQuery, actorParams); err != nil {
		return err
	}

	userQuery := `
		INSERT INTO users (id, username, email, password_hash, role)
		VALUES (:id, :username, :email, :password_hash, :role)
	`
	if _, err := tx.NamedExecContext(ctx, userQuery, user); err != nil {
		return err
	}
	if err := tx.QueryRowxContext(ctx, `
		SELECT created_at, updated_at FROM users WHERE id = $1
	`, user.ID).Scan(&user.CreatedAt, &user.UpdatedAt); err != nil {
		return err
	}

	keyQuery := `
		INSERT INTO actor_keys (actor_id, key_id, public_key_pem, private_key_pem)
		VALUES (:actor_id, :key_id, :public_key_pem, :private_key_pem)
	`
	if _, err := tx.NamedExecContext(ctx, keyQuery, map[string]any{
		"actor_id":        user.ID,
		"key_id":          activitypub.KeyID(user.APID),
		"public_key_pem":  user.PublicKeyPEM,
		"private_key_pem": user.PrivateKeyPEM,
	}); err != nil {
		return err
	}

	doc := activitypub.ActorDocument("Person", user.APID, user.Username, user.Name, user.Summary, user.PublicKeyPEM)
	rawDoc, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	objectQuery := `
		INSERT INTO ap_objects (ap_id, object_type, actor_id, local_ref_table, local_ref_id, document)
		VALUES ($1, 'Person', $2, 'users', $3, $4)
	`
	if _, err := tx.ExecContext(ctx, objectQuery, user.APID, user.ID, user.ID, rawDoc); err != nil {
		return err
	}

	return nil
}

// GetUserByEmail finds user by email
func (r *PgRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {

	var user User

	query := `
		SELECT
			u.id::text,
			a.ap_id,
			u.username,
			u.email,
			u.password_hash,
			u.role,
			a.handle,
			a.name,
			a.summary,
			u.created_at,
			u.updated_at
		FROM users u
		JOIN actors a ON a.id = u.id
		WHERE u.email = $1
	`

	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
