package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Cluster struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ClusterStore struct {
	pool *pgxpool.Pool
}

func NewClusterStore(pool *pgxpool.Pool) *ClusterStore {
	return &ClusterStore{pool: pool}
}

func (s *ClusterStore) CreateCluster(ctx context.Context, c *Cluster) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO clusters (name)
		VALUES ($1)
		RETURNING id, created_at, updated_at
	`, c.Name).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (s *ClusterStore) GetCluster(ctx context.Context, id uuid.UUID) (*Cluster, error) {
	var c Cluster
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, created_at, updated_at
		FROM clusters WHERE id = $1
	`, id).Scan(&c.ID, &c.Name, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &c, err
}
