package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MailboxStatus string

const (
	MailboxStatusActive    MailboxStatus = "active"
	MailboxStatusSuspended MailboxStatus = "suspended"
)

type Mailbox struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	Domain        string
	Address       string
	PasswordHash  string // {BLF-CRYPT} bcrypt hash
	QuotaMB       int
	Status        MailboxStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MailAlias struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	Domain        string
	Source        string
	Destination   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type MailStore struct {
	pool *pgxpool.Pool
}

func NewMailStore(pool *pgxpool.Pool) *MailStore {
	return &MailStore{pool: pool}
}

func (s *MailStore) CreateMailbox(ctx context.Context, m *Mailbox) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO mailboxes (hosting_user_id, domain, address, password_hash, quota_mb, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, m.HostingUserID, m.Domain, m.Address, m.PasswordHash, m.QuotaMB, m.Status).Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
}

func (s *MailStore) GetMailbox(ctx context.Context, id uuid.UUID) (*Mailbox, error) {
	var m Mailbox
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, domain, address, password_hash, quota_mb, status, created_at, updated_at
		FROM mailboxes WHERE id = $1
	`, id).Scan(&m.ID, &m.HostingUserID, &m.Domain, &m.Address, &m.PasswordHash, &m.QuotaMB, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &m, err
}

func (s *MailStore) DeleteMailbox(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM mailboxes WHERE id = $1`, id)
	return err
}

func (s *MailStore) CreateAlias(ctx context.Context, a *MailAlias) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO mail_aliases (hosting_user_id, domain, source, destination)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`, a.HostingUserID, a.Domain, a.Source, a.Destination).Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
}

func (s *MailStore) DeleteAlias(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM mail_aliases WHERE id = $1`, id)
	return err
}
