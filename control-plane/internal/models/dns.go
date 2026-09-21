package models

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DNSZoneStatus string

const (
	DNSZoneStatusActive    DNSZoneStatus = "active"
	DNSZoneStatusSuspended DNSZoneStatus = "suspended"
	DNSZoneStatusDeleting  DNSZoneStatus = "deleting"
	DNSZoneStatusFailed    DNSZoneStatus = "failed"
)

type DNSZone struct {
	ID            uuid.UUID
	HostingUserID uuid.UUID
	Domain        string
	Status        DNSZoneStatus
	Serial        int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type DNSRecord struct {
	ID        uuid.UUID
	ZoneID    uuid.UUID
	Name      string
	Type      string
	Content   string
	TTL       int
	Priority  *int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type DNSStore struct {
	pool *pgxpool.Pool
}

func NewDNSStore(pool *pgxpool.Pool) *DNSStore {
	return &DNSStore{pool: pool}
}

// CreateZone inserts a new DNS zone record.
func (s *DNSStore) CreateZone(ctx context.Context, z *DNSZone) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO dns_zones (hosting_user_id, domain, status, serial)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
	`, z.HostingUserID, z.Domain, z.Status, z.Serial).Scan(&z.ID, &z.CreatedAt, &z.UpdatedAt)
}

// GetZone retrieves a DNS zone by ID.
func (s *DNSStore) GetZone(ctx context.Context, id uuid.UUID) (*DNSZone, error) {
	var z DNSZone
	err := s.pool.QueryRow(ctx, `
		SELECT id, hosting_user_id, domain, status, serial, created_at, updated_at
		FROM dns_zones WHERE id = $1
	`, id).Scan(&z.ID, &z.HostingUserID, &z.Domain, &z.Status, &z.Serial, &z.CreatedAt, &z.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &z, err
}

// IncrementSerial increments the SOA serial number of the zone and returns it.
func (s *DNSStore) IncrementSerial(ctx context.Context, zoneID uuid.UUID) (int64, error) {
	var serial int64
	err := s.pool.QueryRow(ctx, `
		UPDATE dns_zones SET serial = serial + 1 WHERE id = $1 RETURNING serial
	`, zoneID).Scan(&serial)
	return serial, err
}

// CreateRecord inserts a new DNS record.
func (s *DNSStore) CreateRecord(ctx context.Context, r *DNSRecord) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO dns_records (zone_id, name, type, content, ttl, priority)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, r.ZoneID, r.Name, r.Type, r.Content, r.TTL, r.Priority).Scan(&r.ID, &r.CreatedAt, &r.UpdatedAt)
}

// ListRecordsByZone retrieves all records for a given zone.
func (s *DNSStore) ListRecordsByZone(ctx context.Context, zoneID uuid.UUID) ([]*DNSRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, zone_id, name, type, content, ttl, priority, created_at, updated_at
		FROM dns_records WHERE zone_id = $1 ORDER BY name, type
	`, zoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*DNSRecord
	for rows.Next() {
		var r DNSRecord
		if err := rows.Scan(&r.ID, &r.ZoneID, &r.Name, &r.Type, &r.Content, &r.TTL, &r.Priority, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}
