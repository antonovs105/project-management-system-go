package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/antonovs105/project-management-system-go/internal/activitypub"
	"github.com/antonovs105/project-management-system-go/internal/adminaudit"
	"github.com/antonovs105/project-management-system-go/internal/secrets"
	"github.com/jmoiron/sqlx"
)

// Repository defines persistence operations for local users and account metadata.
type Repository interface {
	CreateUser(ctx context.Context, user *User) error
	CreateUserWithOAuthIdentity(ctx context.Context, user *User, identity *OAuthIdentity) error
	CreateAdminIfNoAdmin(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, userID string) (*User, error)
	GetOAuthIdentity(ctx context.Context, provider, subject string) (*OAuthIdentity, error)
	UpdateOAuthIdentity(ctx context.Context, identity *OAuthIdentity) error
	CreateOAuthLoginCode(ctx context.Context, userID, codeHash string, expiresAt time.Time) error
	ConsumeOAuthLoginCode(ctx context.Context, codeHash string, now time.Time) (*User, error)
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
	TokenVersion(ctx context.Context, userID string) (int, error)
	InstanceRole(ctx context.Context, userID string) (string, error)
	ListUsers(ctx context.Context, options ListUsersOptions) ([]User, error)
	UpdateInstanceRole(ctx context.Context, adminUserID, userID, role string) (*User, error)
}

// PgRepository implements Repository using PostgreSQL.
type PgRepository struct {
	db              *sqlx.DB
	cfg             activitypub.Config
	privateKeyCodec secrets.PrivateKeyCodec
}

// NewRepository creates a PostgreSQL-backed user repository.
func NewRepository(db *sqlx.DB, cfg activitypub.Config, codecs ...secrets.PrivateKeyCodec) Repository {
	var privateKeyCodec secrets.PrivateKeyCodec = secrets.NoopPrivateKeyCodec{}
	if len(codecs) > 0 && codecs[0] != nil {
		privateKeyCodec = codecs[0]
	}
	return &PgRepository{
		db:              db,
		cfg:             cfg,
		privateKeyCodec: privateKeyCodec,
	}
}

// CreateUser stores a regular user and its ActivityPub actor graph.
func (r *PgRepository) CreateUser(ctx context.Context, user *User) error {
	return r.createUserInTx(ctx, user, nil)
}

// CreateUserWithOAuthIdentity stores a regular user and links an OAuth identity atomically.
func (r *PgRepository) CreateUserWithOAuthIdentity(ctx context.Context, user *User, identity *OAuthIdentity) error {
	return r.createUserInTx(ctx, user, nil, identity)
}

// CreateAdminIfNoAdmin stores the first owner when no owner currently exists.
func (r *PgRepository) CreateAdminIfNoAdmin(ctx context.Context, user *User) error {
	return r.createUserInTx(ctx, user, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('admin-bootstrap'))`); err != nil {
			return err
		}

		var exists bool
		if err := tx.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM users WHERE instance_role = $1)`, InstanceRoleOwner); err != nil {
			return err
		}
		if exists {
			return ErrAdminAlreadyExists
		}
		return nil
	})
}

// createUserInTx runs the optional guard and inserts the full user actor graph atomically.
func (r *PgRepository) createUserInTx(ctx context.Context, user *User, beforeInsert func(*sqlx.Tx) error, identities ...*OAuthIdentity) error {
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
	for _, identity := range identities {
		if identity == nil {
			continue
		}
		identity.UserID = user.ID
		if err := insertOAuthIdentity(ctx, tx, identity); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// insertUserGraph writes the actor, user, key, and JSON-LD object rows for a user.
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
		INSERT INTO users (id, username, email, password_hash, instance_role)
		VALUES (:id, :username, :email, :password_hash, :instance_role)
	`
	if _, err := tx.NamedExecContext(ctx, userQuery, user); err != nil {
		return err
	}
	if err := tx.QueryRowxContext(ctx, `
		SELECT token_version, created_at, updated_at FROM users WHERE id = $1
	`, user.ID).Scan(&user.TokenVersion, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return err
	}

	keyQuery := `
		INSERT INTO actor_keys (actor_id, key_id, public_key_pem, private_key_pem)
		VALUES (:actor_id, :key_id, :public_key_pem, :private_key_pem)
	`
	storedPrivateKey, err := r.privateKeyCodec.EncryptPrivateKey(user.PrivateKeyPEM)
	if err != nil {
		return err
	}
	if _, err := tx.NamedExecContext(ctx, keyQuery, map[string]any{
		"actor_id":        user.ID,
		"key_id":          activitypub.KeyID(user.APID),
		"public_key_pem":  user.PublicKeyPEM,
		"private_key_pem": storedPrivateKey,
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

// GetUserByEmail finds a local user by email and includes password hash data.
func (r *PgRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {

	var user User

	query := `
		SELECT
			u.id::text,
			a.ap_id,
			u.username,
			u.email,
			u.password_hash,
			u.instance_role,
			u.token_version,
			a.handle,
			a.name,
			a.summary,
			u.created_at,
			u.updated_at
		FROM users u
		JOIN actors a ON a.id = u.id
		WHERE lower(u.email) = lower($1)
	`

	err := r.db.GetContext(ctx, &user, query, email)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByID finds a local user by UUID and includes password hash data.
func (r *PgRepository) GetUserByID(ctx context.Context, userID string) (*User, error) {
	var user User
	err := r.db.GetContext(ctx, &user, userSelectWithPasswordQuery()+`
		WHERE u.id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetOAuthIdentity finds a linked provider account.
func (r *PgRepository) GetOAuthIdentity(ctx context.Context, provider, subject string) (*OAuthIdentity, error) {
	var identity OAuthIdentity
	if err := r.db.GetContext(ctx, &identity, `
		SELECT
			id::text,
			user_id::text,
			provider,
			provider_subject,
			email,
			email_verified,
			display_name,
			avatar_url,
			created_at,
			updated_at
		FROM oauth_identities
		WHERE provider = $1 AND provider_subject = $2
	`, provider, subject); err != nil {
		return nil, err
	}
	return &identity, nil
}

// UpdateOAuthIdentity refreshes provider metadata for an existing link.
func (r *PgRepository) UpdateOAuthIdentity(ctx context.Context, identity *OAuthIdentity) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE oauth_identities
		SET email = $3,
			email_verified = $4,
			display_name = $5,
			avatar_url = $6,
			updated_at = now()
		WHERE provider = $1 AND provider_subject = $2
	`, identity.Provider, identity.ProviderSubject, identity.Email, identity.EmailVerified, identity.DisplayName, identity.AvatarURL)
	return err
}

// CreateOAuthLoginCode stores a short-lived one-time frontend exchange code.
func (r *PgRepository) CreateOAuthLoginCode(ctx context.Context, userID, codeHash string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO oauth_login_codes (user_id, code_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, codeHash, expiresAt)
	return err
}

// ConsumeOAuthLoginCode marks a one-time code used and returns the linked user.
func (r *PgRepository) ConsumeOAuthLoginCode(ctx context.Context, codeHash string, now time.Time) (*User, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var userID string
	if err := tx.GetContext(ctx, &userID, `
		UPDATE oauth_login_codes
		SET used_at = $2
		WHERE code_hash = $1
			AND used_at IS NULL
			AND expires_at > $2
		RETURNING user_id::text
	`, codeHash, now); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrOAuthInvalidCode
		}
		return nil, err
	}
	user, err := loadUserByID(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return user, nil
}

// UpdatePasswordHash changes a user's password hash and bumps token invalidation state.
func (r *PgRepository) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $2,
			token_version = token_version + 1
		WHERE id = $1
	`, userID, passwordHash)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// insertOAuthIdentity stores a provider identity link inside an existing transaction.
func insertOAuthIdentity(ctx context.Context, tx *sqlx.Tx, identity *OAuthIdentity) error {
	if _, err := tx.NamedExecContext(ctx, `
		INSERT INTO oauth_identities (
			user_id, provider, provider_subject, email, email_verified, display_name, avatar_url
		)
		VALUES (
			:user_id, :provider, :provider_subject, :email, :email_verified, :display_name, :avatar_url
		)
	`, identity); err != nil {
		return err
	}
	return nil
}

// TokenVersion returns the current JWT invalidation version for a local user.
func (r *PgRepository) TokenVersion(ctx context.Context, userID string) (int, error) {
	var tokenVersion int
	err := r.db.GetContext(ctx, &tokenVersion, `SELECT token_version FROM users WHERE id = $1`, userID)
	return tokenVersion, err
}

// InstanceRole returns the instance role for a local user.
func (r *PgRepository) InstanceRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := r.db.GetContext(ctx, &role, `SELECT instance_role FROM users WHERE id = $1`, userID)
	return role, err
}

// ListUsers returns local users that match admin filters and pagination.
func (r *PgRepository) ListUsers(ctx context.Context, options ListUsersOptions) ([]User, error) {
	users := make([]User, 0)
	if err := r.db.SelectContext(ctx, &users, userSelectQuery()+`
		WHERE ($1 = '' OR u.instance_role = $1)
			AND (
				$2 = ''
				OR u.username ILIKE '%' || $2 || '%'
				OR u.email ILIKE '%' || $2 || '%'
				OR a.handle ILIKE '%' || $2 || '%'
				OR a.name ILIKE '%' || $2 || '%'
			)
		ORDER BY u.created_at DESC, lower(u.username) ASC
		LIMIT $3 OFFSET $4
	`, options.InstanceRole, options.Query, options.Limit, options.Offset); err != nil {
		return nil, err
	}
	return users, nil
}

// UpdateInstanceRole updates an instance user role and audits the change.
func (r *PgRepository) UpdateInstanceRole(ctx context.Context, adminUserID, userID, role string) (*User, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var currentRole string
	if err := tx.GetContext(ctx, &currentRole, `SELECT instance_role FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	var actorRole string
	if err := tx.GetContext(ctx, &actorRole, `SELECT instance_role FROM users WHERE id = $1`, adminUserID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrAdminRequired
		}
		return nil, err
	}

	if actorRole != InstanceRoleOwner && (currentRole == InstanceRoleOwner || role == InstanceRoleOwner) {
		return nil, ErrOwnerRequired
	}

	if currentRole == InstanceRoleOwner && role != InstanceRoleOwner {
		var adminCount int
		if err := tx.GetContext(ctx, &adminCount, `SELECT count(*) FROM users WHERE instance_role = $1`, InstanceRoleOwner); err != nil {
			return nil, err
		}
		if adminCount <= 1 {
			return nil, ErrCannotDemoteLastAdmin
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE users SET instance_role = $2 WHERE id = $1`, userID, role); err != nil {
		return nil, err
	}
	if currentRole != role {
		if _, err := adminaudit.InsertEvent(ctx, tx, adminaudit.EventInput{
			ActorUserID: adminUserID,
			Action:      adminaudit.ActionUserInstanceRoleUpdated,
			TargetType:  adminaudit.TargetTypeUser,
			TargetID:    userID,
			Metadata: map[string]any{
				"old_instance_role": currentRole,
				"new_instance_role": role,
			},
		}); err != nil {
			return nil, err
		}
	}

	updated, err := loadUserByID(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

// loadUserByID loads a user using the public admin response projection.
func loadUserByID(ctx context.Context, q sqlx.QueryerContext, userID string) (*User, error) {
	var user User
	if err := sqlx.GetContext(ctx, q, &user, userSelectQuery()+`
		WHERE u.id = $1
	`, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// userSelectQuery returns the shared SQL projection for user API responses.
func userSelectQuery() string {
	return `
		SELECT
			u.id::text,
			a.ap_id,
			u.username,
			u.email,
			u.instance_role,
			u.token_version,
			a.handle,
			a.name,
			a.summary,
			u.created_at,
			u.updated_at
		FROM users u
		JOIN actors a ON a.id = u.id
	`
}

// userSelectWithPasswordQuery returns the shared SQL projection used by authentication flows.
func userSelectWithPasswordQuery() string {
	return `
		SELECT
			u.id::text,
			a.ap_id,
			u.username,
			u.email,
			u.password_hash,
			u.instance_role,
			u.token_version,
			a.handle,
			a.name,
			a.summary,
			u.created_at,
			u.updated_at
		FROM users u
		JOIN actors a ON a.id = u.id
	`
}
