// Package models provides database access models.
package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeStatus represents the operational state of a node.
type NodeStatus string

const (
	NodeStatusOnline      NodeStatus = "online"
	NodeStatusOffline     NodeStatus = "offline"
	NodeStatusMaintenance NodeStatus = "maintenance"
)

// Node represents a hosting server (agent) in the cluster.
type Node struct {
	ID                  uuid.UUID
	Name                string
	Address             string
	Role                string
	ClusterID           *uuid.UUID
	SigningKeyEncrypted string // AES-256-GCM encrypted
	Status              NodeStatus
	Version             *string
	LastSeenAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// NodeStore handles database operations for Nodes.
type NodeStore struct {
	pool *pgxpool.Pool
}

// NewNodeStore creates a new NodeStore.
func NewNodeStore(pool *pgxpool.Pool) *NodeStore {
	return &NodeStore{pool: pool}
}

// GetByID retrieves a node by its ID.
func (s *NodeStore) GetByID(ctx context.Context, id uuid.UUID) (*Node, error) {
	var n Node
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, address, role, cluster_id, signing_key_encrypted, status, version, last_seen_at, created_at, updated_at
		FROM nodes
		WHERE id = $1
	`, id).Scan(
		&n.ID, &n.Name, &n.Address, &n.Role, &n.ClusterID, &n.SigningKeyEncrypted, &n.Status,
		&n.Version, &n.LastSeenAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil // not found
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// UpdateLastSeen updates the node's last_seen_at timestamp and sets status to online.
func (s *NodeStore) UpdateLastSeen(ctx context.Context, id uuid.UUID, version string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE nodes
		SET last_seen_at = now(), status = 'online', version = $2
		WHERE id = $1
	`, id, version)
	return err
}

// ListAll returns all nodes.
func (s *NodeStore) ListAll(ctx context.Context) ([]*Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, address, role, cluster_id, signing_key_encrypted, status, version, last_seen_at, created_at, updated_at
		FROM nodes
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.Name, &n.Address, &n.Role, &n.ClusterID, &n.SigningKeyEncrypted, &n.Status,
			&n.Version, &n.LastSeenAt, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, &n)
	}
	return nodes, rows.Err()
}

// CreateNode inserts a new node into the database.
func (s *NodeStore) CreateNode(ctx context.Context, n *Node) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO nodes (name, address, role, cluster_id, signing_key_encrypted, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, n.Name, n.Address, n.Role, n.ClusterID, n.SigningKeyEncrypted, n.Status).Scan(&n.ID, &n.CreatedAt, &n.UpdatedAt)
}

func (s *NodeStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, "UPDATE nodes SET status = $1, updated_at = now() WHERE id = $2", status, id)
	return err
}

// GetNodesByCluster retrieves nodes for a specific cluster.
func (s *NodeStore) GetNodesByCluster(ctx context.Context, clusterID uuid.UUID) ([]*Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, address, role, cluster_id, signing_key_encrypted, status, version, last_seen_at, created_at, updated_at
		FROM nodes WHERE cluster_id = $1
	`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		var n Node
		if err := rows.Scan(&n.ID, &n.Name, &n.Address, &n.Role, &n.ClusterID, &n.SigningKeyEncrypted, &n.Status, &n.Version, &n.LastSeenAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, &n)
	}
	return nodes, nil
}
