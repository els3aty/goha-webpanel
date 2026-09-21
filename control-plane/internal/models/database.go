package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DatabaseStatus string

const (
	DatabaseStatusActive   DatabaseStatus = "active"
	DatabaseStatusDeleting DatabaseStatus = "deleting"
	DatabaseStatusFailed   DatabaseStatus = "failed"
)

type Database struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	DBName        string
	Status        DatabaseStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DatabaseUser struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	DBUsername    string
	Status        DatabaseStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DatabaseStore struct {
	pool *pgxpool.Pool
}

func NewDatabaseStore(pool *pgxpool.Pool) *DatabaseStore {
	return &DatabaseStore{pool: pool}
}

// CreateDatabase inserts a new database record.
func (s *DatabaseStore) CreateDatabase(ctx context.Context, db *Database) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO databases (hosting_user_id, db_name, status)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`, db.HostingUserID, db.DBName, db.Status).Scan(&db.ID, &db.CreatedAt, &db.UpdatedAt)
}

// GetDatabase retrieves a database by ID.
func (s *DatabaseStore) GetDatabase(ctx context.Context, id uuid.UUID) (*Database, error) {
	var db Database
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, db_name, status, created_at, updated_at
		FROM databases WHERE id = $1
	`, id).Scan(&db.ID, &db.HostingUserID, &db.DBName, &db.Status, &db.CreatedAt, &db.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &db, err
}

// CreateDatabaseUser inserts a new DB user record.
func (s *DatabaseStore) CreateDatabaseUser(ctx context.Context, u *DatabaseUser) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO database_users (hosting_user_id, db_username, status)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`, u.HostingUserID, u.DBUsername, u.Status).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

// GetDatabaseUser retrieves a db user by ID.
func (s *DatabaseStore) GetDatabaseUser(ctx context.Context, id uuid.UUID) (*DatabaseUser, error) {
	var u DatabaseUser
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, db_username, status, created_at, updated_at
		FROM database_users WHERE id = $1
	`, id).Scan(&u.ID, &u.HostingUserID, &u.DBUsername, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

// GrantPrivileges links a user to a database with privileges.
func (s *DatabaseStore) GrantPrivileges(ctx context.Context, dbID, userID uuid.UUID, privileges string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO database_grants (database_id, database_user_id, privileges)
		VALUES ($1, $2, $3)
		ON CONFLICT (database_id, database_user_id) DO UPDATE SET privileges = EXCLUDED.privileges
	`, dbID, userID, privileges)
	return err
}
