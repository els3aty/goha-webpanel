package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HostingPackage struct {
	ID            uuid.UUID
	Name          string
	DiskQuotaMB   int
	MaxDomains    int
	MaxDatabases  int
	MaxMailboxes  int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type PackageStore struct {
	pool *pgxpool.Pool
}

func NewPackageStore(pool *pgxpool.Pool) *PackageStore {
	return &PackageStore{pool: pool}
}

func (s *PackageStore) CreatePackage(ctx context.Context, p *HostingPackage) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO hosting_packages (name, disk_quota_mb, max_domains, max_databases, max_mailboxes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
	`, p.Name, p.DiskQuotaMB, p.MaxDomains, p.MaxDatabases, p.MaxMailboxes).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

func (s *PackageStore) GetPackage(ctx context.Context, id uuid.UUID) (*HostingPackage, error) {
	var p HostingPackage
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, disk_quota_mb, max_domains, max_databases, max_mailboxes, created_at, updated_at
		FROM hosting_packages WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.DiskQuotaMB, &p.MaxDomains, &p.MaxDatabases, &p.MaxMailboxes, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &p, err
}

func (s *PackageStore) ListPackages(ctx context.Context) ([]HostingPackage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, disk_quota_mb, max_domains, max_databases, max_mailboxes, created_at, updated_at
		FROM hosting_packages ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pkgs []HostingPackage
	for rows.Next() {
		var p HostingPackage
		if err := rows.Scan(&p.ID, &p.Name, &p.DiskQuotaMB, &p.MaxDomains, &p.MaxDatabases, &p.MaxMailboxes, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}
