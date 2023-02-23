package pgstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultClientStoreTable is the default collection for storing clients.
	DefaultClientStoreTable = "oauth2_clients"
)

// ClientStoreOption is a function that configures the ClientStore.
type ClientStoreOption func(*ClientStore) error

// WithClientStoreTable configures the auth token table.
func WithClientStoreTable(table string) ClientStoreOption {
	return func(s *ClientStore) error {
		if table == "" {
			return ErrNoTable
		}

		s.table = table

		return nil
	}
}

// WithClientStoreConnPool configures the connection pool.
func WithClientStoreConnPool(pool *pgxpool.Pool) ClientStoreOption {
	return func(s *ClientStore) error {
		if pool == nil {
			return ErrNoConnPool
		}

		s.pool = pool

		return nil
	}
}

// WithClientStoreLogger configures the logger.
func WithClientStoreLogger(logger Logger) ClientStoreOption {
	return func(s *ClientStore) error {
		if logger == nil {
			return ErrNoLogger
		}

		s.logger = logger

		return nil
	}
}

// ClientStoreItem data item
type ClientStoreItem struct {
	ID        int64     `db:"id"`
	Secret    string    `db:"secret"`
	Domain    string    `db:"domain"`
	Data      []byte    `db:"data"`
	CreatedAt time.Time `db:"created_at"`
}

// ClientStore is a data struct that stores oauth2 client information.
type ClientStore struct {
	pool   *pgxpool.Pool
	table  string
	logger Logger
}

// scanToClientInfo scans a row into an oauth2.ClientInfo.
func (s *ClientStore) scanToClientInfo(ctx context.Context, row pgx.Row) (oauth2.ClientInfo, error) {
	var item ClientStoreItem
	err := row.Scan(&item.ID, &item.Secret, &item.Domain, &item.Data, &item.CreatedAt)
	if err != nil {
		return nil, err
	}

	var info oauth2.ClientInfo
	err = json.Unmarshal(item.Data, &info)
	if err != nil {
		return nil, err
	}

	s.logger.Log(ctx, LogLevelDebug, "client found", "id", item.ID)

	return info, nil
}

// InitTable initializes the client store table if it does not exist and
// creates the indexes.
func (s *ClientStore) InitTable(ctx context.Context) error {
	s.logger.Log(ctx, LogLevelDebug, "initializing client store table", "table", s.table)

	_, err := s.pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %[1]s (
			id     VARCHAR(255) PRIMARY KEY,
			secret VARCHAR(255) NOT NULL,
			domain VARCHAR(255) NOT NULL,
			data   JSONB NOT NULL
			created_at    TIMESTAMPTZ NOT NULL,
		);

		CREATE INDEX IF NOT EXISTS %[1]s_domain_idx ON %[1]s (domain);`,
		s.table,
	))

	if err != nil {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return err
	}

	return nil
}

// Create creates a new client in the store.
func (s *ClientStore) Create(info oauth2.ClientInfo) error {
	s.logger.Log(context.Background(), LogLevelDebug, "creating client", "id", info.GetID())
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(context.Background(), fmt.Sprintf(`
		INSERT INTO %[1]s (id, secret, domain, data, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		s.table,
	), info.GetID(), info.GetSecret(), info.GetDomain(), data, time.Now())

	if err != nil {
		s.logger.Log(context.Background(), LogLevelError, "creating client failed", "info", info)
		return err
	}

	s.logger.Log(context.Background(), LogLevelDebug, "client created")

	return nil
}

// GetByID returns the client information by key from the store.
func (s *ClientStore) GetByID(ctx context.Context, id string) (oauth2.ClientInfo, error) {
	s.logger.Log(ctx, LogLevelDebug, "client get by id", "id", id)
	row := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT * FROM %s WHERE id = $1", s.table), id)
	return s.scanToClientInfo(ctx, row)
}

// NewClientStore creates a new ClientStore.
func NewClientStore(opts ...ClientStoreOption) (*ClientStore, error) {
	s := &ClientStore{
		table:  DefaultClientStoreTable,
		logger: new(NoopLogger),
	}

	for _, o := range opts {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	if s.pool == nil {
		return nil, ErrNoConnPool
	}

	return s, nil
}
