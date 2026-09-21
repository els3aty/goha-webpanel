package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type NodeAppStatus string

const (
	NodeAppStatusStopped NodeAppStatus = "stopped"
	NodeAppStatusRunning NodeAppStatus = "running"
	NodeAppStatusFailed  NodeAppStatus = "failed"
)

type NodeApp struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	AppName       string
	Domain        string
	AppPath       string
	StartupFile   string
	NodeVersion   string
	Port          int
	Status        NodeAppStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NodeAppStore struct {
	pool *pgxpool.Pool
}

func NewNodeAppStore(pool *pgxpool.Pool) *NodeAppStore {
	return &NodeAppStore{pool: pool}
}

// CreateApp inserts a new Node.js app record.
func (s *NodeAppStore) CreateApp(ctx context.Context, app *NodeApp) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO nodejs_apps (hosting_user_id, app_name, domain, app_path, startup_file, node_version, port, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, app.HostingUserID, app.AppName, app.Domain, app.AppPath, app.StartupFile, app.NodeVersion, app.Port, app.Status).Scan(&app.ID, &app.CreatedAt, &app.UpdatedAt)
}

// GetApp retrieves an app by ID.
func (s *NodeAppStore) GetApp(ctx context.Context, id uuid.UUID) (*NodeApp, error) {
	var app NodeApp
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, app_name, domain, app_path, startup_file, node_version, port, status, created_at, updated_at
		FROM nodejs_apps WHERE id = $1
	`, id).Scan(&app.ID, &app.HostingUserID, &app.AppName, &app.Domain, &app.AppPath, &app.StartupFile, &app.NodeVersion, &app.Port, &app.Status, &app.CreatedAt, &app.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &app, err
}

// UpdateAppStatus updates the status of an app.
func (s *NodeAppStore) UpdateAppStatus(ctx context.Context, id uuid.UUID, status NodeAppStatus) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE nodejs_apps SET status = $1 WHERE id = $2
	`, status, id)
	return err
}

// DeleteApp removes an app from the DB.
func (s *NodeAppStore) DeleteApp(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM nodejs_apps WHERE id = $1`, id)
	return err
}
