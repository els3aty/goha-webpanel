package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BackupStatus string
type BackupType string

const (
	BackupStatusPending   BackupStatus = "pending"
	BackupStatusRunning   BackupStatus = "running"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"

	BackupTypeFiles    BackupType = "files"
	BackupTypeDatabase BackupType = "database"
	BackupTypeFull     BackupType = "full"
)

type Backup struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	BackupType    BackupType
	FilePath      string
	SizeBytes     int64
	Status        BackupStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type BackupStore struct {
	pool *pgxpool.Pool
}

func NewBackupStore(pool *pgxpool.Pool) *BackupStore {
	return &BackupStore{pool: pool}
}

// CreateBackup inserts a new backup record.
func (s *BackupStore) CreateBackup(ctx context.Context, b *Backup) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO backups (hosting_user_id, backup_type, file_path, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`, b.HostingUserID, b.BackupType, b.FilePath, b.Status).Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
}

// UpdateBackupStatus updates the status and size of a backup.
func (s *BackupStore) UpdateBackupStatus(ctx context.Context, id uuid.UUID, status BackupStatus, sizeBytes int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE backups SET status = $1, size_bytes = $2 WHERE id = $3
	`, status, sizeBytes, id)
	return err
}

// GetBackup retrieves a backup by ID.
func (s *BackupStore) GetBackup(ctx context.Context, id uuid.UUID) (*Backup, error) {
	var b Backup
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, backup_type, file_path, size_bytes, status, created_at, updated_at
		FROM backups WHERE id = $1
	`, id).Scan(&b.ID, &b.HostingUserID, &b.BackupType, &b.FilePath, &b.SizeBytes, &b.Status, &b.CreatedAt, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &b, err
}
