package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WHMCSToken struct {
	ID         uuid.UUID
	Name       string
	TokenHash  string
	AllowedIPs []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	UpdatedAt  time.Time
}

type WHMCSStore struct {
	pool *pgxpool.Pool
}

func NewWHMCSStore(pool *pgxpool.Pool) *WHMCSStore {
	return &WHMCSStore{pool: pool}
}

// GetTokenByHash retrieves a WHMCS token by its hash.
func (s *WHMCSStore) GetTokenByHash(ctx context.Context, hash string) (*WHMCSToken, error) {
	var token WHMCSToken
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, token_hash, allowed_ips, created_at, last_used_at, updated_at
		FROM whmcs_api_tokens WHERE token_hash = $1
	`, hash).Scan(&token.ID, &token.Name, &token.TokenHash, &token.AllowedIPs, &token.CreatedAt, &token.LastUsedAt, &token.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &token, err
}

// UpdateLastUsed updates the last_used_at timestamp.
func (s *WHMCSStore) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE whmcs_api_tokens SET last_used_at = now() WHERE id = $1
	`, id)
	return err
}

// CreateToken inserts a new WHMCS token.
func (s *WHMCSStore) CreateToken(ctx context.Context, token *WHMCSToken) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO whmcs_api_tokens (name, token_hash, allowed_ips)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`, token.Name, token.TokenHash, token.AllowedIPs).Scan(&token.ID, &token.CreatedAt, &token.UpdatedAt)
}
