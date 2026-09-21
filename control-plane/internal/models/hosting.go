package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HostingUserStatus string

const (
	HostingUserStatusActive   HostingUserStatus = "active"
	HostingUserStatusLocked   HostingUserStatus = "locked"
	HostingUserStatusDeleting HostingUserStatus = "deleting"
	HostingUserStatusFailed   HostingUserStatus = "failed"
)

type VirtualHostStatus string

const (
	VHostStatusActive    VirtualHostStatus = "active"
	VHostStatusSuspended VirtualHostStatus = "suspended"
	VHostStatusDeleting  VirtualHostStatus = "deleting"
	VHostStatusFailed    VirtualHostStatus = "failed"
)

type HostingUser struct {
	ID         uuid.UUID
	CustomerID uuid.UUID
	NodeID     uuid.UUID
	PackageID  uuid.UUID
	Username   string
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type VirtualHost struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	Domain        string
	DocumentRoot  string
	RuntimeType   string
	RuntimePort   *int
	PHPVersion    *string
	SSLEnabled    bool
	Status        VirtualHostStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type HostingStore struct {
	pool *pgxpool.Pool
}

func NewHostingStore(pool *pgxpool.Pool) *HostingStore {
	return &HostingStore{pool: pool}
}

// CreateHostingUser inserts a new hosting user into the database.
func (s *HostingStore) CreateHostingUser(ctx context.Context, u *HostingUser) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO hosting_users (customer_id, node_id, package_id, username, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, u.CustomerID, u.NodeID, u.PackageID, u.Username, u.Status).Scan(&u.ID, &u.CreatedAt, &u.UpdatedAt)
}

// GetHostingUser retrieves a hosting user by ID.
func (s *HostingStore) GetHostingUser(ctx context.Context, id uuid.UUID) (*HostingUser, error) {
	var u HostingUser
	err := s.pool.QueryRow(ctx, `
		SELECT id, customer_id, node_id, package_id, username, status, created_at, updated_at
		FROM hosting_users WHERE id = $1
	`, id).Scan(&u.ID, &u.CustomerID, &u.NodeID, &u.PackageID, &u.Username, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

// CreateVirtualHost inserts a new virtual host into the database.
func (s *HostingStore) CreateVirtualHost(ctx context.Context, v *VirtualHost) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO virtual_hosts (hosting_user_id, domain, document_root, runtime_type, runtime_port, php_version, ssl_enabled, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`, v.HostingUserID, v.Domain, v.DocumentRoot, v.RuntimeType, v.RuntimePort, v.PHPVersion, v.SSLEnabled, v.Status).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)
}

// GetVirtualHost retrieves a virtual host by domain.
func (s *HostingStore) GetVirtualHost(ctx context.Context, domain string) (*VirtualHost, error) {
	var v VirtualHost
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, domain, document_root, runtime_type, runtime_port, php_version, ssl_enabled, status, created_at, updated_at
		FROM virtual_hosts WHERE domain = $1
	`, domain).Scan(&v.ID, &v.HostingUserID, &v.Domain, &v.DocumentRoot, &v.RuntimeType, &v.RuntimePort, &v.PHPVersion, &v.SSLEnabled, &v.Status, &v.CreatedAt, &v.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

// GetVirtualHostByID retrieves a virtual host by ID.
func (s *HostingStore) GetVirtualHostByID(ctx context.Context, id uuid.UUID) (*VirtualHost, error) {
	var v VirtualHost
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, domain, document_root, runtime_type, runtime_port, php_version, ssl_enabled, status, created_at, updated_at
		FROM virtual_hosts WHERE id = $1
	`, id).Scan(&v.ID, &v.HostingUserID, &v.Domain, &v.DocumentRoot, &v.RuntimeType, &v.RuntimePort, &v.PHPVersion, &v.SSLEnabled, &v.Status, &v.CreatedAt, &v.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

// EnableSSL marks the virtual host as having SSL enabled.
func (s *HostingStore) EnableSSL(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE virtual_hosts SET ssl_enabled = true WHERE id = $1
	`, id)
	return err
}
