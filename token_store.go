package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// DefaultTokenStoreTable is the default collection for storing tokens.
	DefaultTokenStoreTable = "oauth2_tokens" // nolint: gosec
)

// TokenStoreOption is a function that configures the TokenStore.
type TokenStoreOption func(*TokenStore) error

// WithTokenStoreTable configures the auth token table.
func WithTokenStoreTable(table string) TokenStoreOption {
	return func(s *TokenStore) error {
		if table == "" {
			return ErrNoTable
		}

		s.table = table

		return nil
	}
}

// WithTokenStoreConnPool configures the connection pool.
func WithTokenStoreConnPool(pool *pgxpool.Pool) TokenStoreOption {
	return func(s *TokenStore) error {
		if pool == nil {
			return ErrNoConnPool
		}

		s.pool = pool

		return nil
	}
}

// WithTokenStoreCleanupInterval configures the cleanup interval.
func WithTokenStoreCleanupInterval(interval time.Duration) TokenStoreOption {
	return func(s *TokenStore) error {
		s.cleanupInterval = interval
		return nil
	}
}

// WithTokenStoreLogger configures the logger.
func WithTokenStoreLogger(logger Logger) TokenStoreOption {
	return func(s *TokenStore) error {
		if logger == nil {
			return ErrNoLogger
		}

		s.logger = logger

		return nil
	}
}

// TokenStoreItem data item
type TokenStoreItem struct {
	ID        int64     `db:"id"`
	Code      string    `db:"code"`
	Access    string    `db:"access_token"`
	Refresh   string    `db:"refresh_token"`
	Data      []byte    `db:"data"`
	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

// TokenStore is a data struct that stores oauth2 token information.
type TokenStore struct {
	pool            *pgxpool.Pool
	table           string
	logger          Logger
	cleanupInterval time.Duration
	cleanupTicker   *time.Ticker
}

// scanToTokenInfo scans a row into an oauth2.TokenInfo.
func (s *TokenStore) scanToTokenInfo(ctx context.Context, row pgx.Row) (oauth2.TokenInfo, error) {
	var item TokenStoreItem
	if err := row.Scan(&item.ID, &item.Code, &item.Access, &item.Refresh, &item.Data, &item.CreatedAt, &item.ExpiresAt); err != nil {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return nil, err
	}

	var info models.Token
	if err := json.Unmarshal(item.Data, &info); err != nil {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return nil, err
	}

	s.logger.Log(ctx, LogLevelDebug, "token found", "id", item.ID)

	return &info, nil
}

// cleanExpiredTokens removes expired tokens from the store.
func (s *TokenStore) cleanExpiredTokens(ctx context.Context) error {
	_, err := s.pool.Query(ctx, fmt.Sprintf("DELETE FROM %s WHERE expires_at <= $1", s.table), time.Now())
	s.logger.Log(ctx, LogLevelDebug, "cleaning expired tokens", "err", err)
	return err
}

// InitCleanup initializes the cleanup process.
func (s *TokenStore) InitCleanup(ctx context.Context) {
	if s.cleanupInterval > 0 {
		s.cleanupTicker = time.NewTicker(s.cleanupInterval)
		go func() {
			for range s.cleanupTicker.C {
				if err := s.cleanExpiredTokens(ctx); err != nil {
					s.logger.Log(ctx, LogLevelError, err.Error())
				}
			}
		}()
	}
}

// InitTable initializes the token store table if it does not exist and creates
// the indexes.
func (s *TokenStore) InitTable(ctx context.Context) error {
	s.logger.Log(ctx, LogLevelDebug, "initializing token store table", "table", s.table)

	_, err := s.pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %[1]s (
			id            BIGSERIAL PRIMARY KEY NOT NULL,
			code          TEXT                  NOT NULL,
			access_token  TEXT                  NOT NULL,
			refresh_token TEXT                  NOT NULL,
			data          JSONB                 NOT NULL,
			created_at    TIMESTAMPTZ           NOT NULL,
			expires_at    TIMESTAMPTZ           NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_%[1]s_code_idx ON %[1]s (code);
		CREATE INDEX IF NOT EXISTS idx_%[1]s_access_idx ON %[1]s (access_token);
		CREATE INDEX IF NOT EXISTS idx_%[1]s_refresh_idx ON %[1]s (refresh_token);
		CREATE INDEX IF NOT EXISTS idx_%[1]s_expires_idx ON %[1]s (expires_at);`,
		s.table,
	))

	if err != nil {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return err
	}

	return nil
}

// Create creates a new token in the store.
func (s *TokenStore) Create(ctx context.Context, info oauth2.TokenInfo) error {
	s.logger.Log(ctx, LogLevelDebug, "creating token", "info", info)

	data, err := json.Marshal(info)
	if err != nil {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return err
	}

	item := TokenStoreItem{
		Data:      data,
		CreatedAt: time.Now(),
	}

	if code := info.GetCode(); code != "" {
		item.Code = code
		item.ExpiresAt = info.GetCodeCreateAt().Add(info.GetCodeExpiresIn())
	} else {
		if access := info.GetAccess(); access != "" {
			item.Access = info.GetAccess()
			item.ExpiresAt = info.GetAccessCreateAt().Add(info.GetAccessExpiresIn())
		}

		if refresh := info.GetRefresh(); refresh != "" {
			item.Refresh = info.GetRefresh()
			item.ExpiresAt = info.GetRefreshCreateAt().Add(info.GetRefreshExpiresIn())
		}
	}

	_, err = s.pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (code, access_token, refresh_token, data, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		s.table,
	), item.Code, item.Access, item.Refresh, item.Data, item.CreatedAt, item.ExpiresAt)

	if err != nil {
		s.logger.Log(ctx, LogLevelError, err.Error(), "info", info, "item", item)
		return err
	}

	s.logger.Log(ctx, LogLevelDebug, "token created")

	return nil
}

// GetByCode returns the token by its authorization code.
func (s *TokenStore) GetByCode(ctx context.Context, code string) (oauth2.TokenInfo, error) {
	s.logger.Log(ctx, LogLevelDebug, "getting token by authorization code", "code", code)
	row := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT * FROM %s WHERE code = $1", s.table), code)
	return s.scanToTokenInfo(ctx, row)
}

// GetByAccess returns the token by its access token.
func (s *TokenStore) GetByAccess(ctx context.Context, access string) (oauth2.TokenInfo, error) {
	s.logger.Log(ctx, LogLevelDebug, "getting token by access token", "access", access)
	row := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT * FROM %s WHERE access_token = $1", s.table), access)
	return s.scanToTokenInfo(ctx, row)
}

// GetByRefresh returns the token by its refresh token.
func (s *TokenStore) GetByRefresh(ctx context.Context, refresh string) (oauth2.TokenInfo, error) {
	s.logger.Log(ctx, LogLevelDebug, "getting token by refresh token", "refresh", refresh)
	row := s.pool.QueryRow(ctx, fmt.Sprintf("SELECT * FROM %s WHERE refresh_token = $1", s.table), refresh)
	return s.scanToTokenInfo(ctx, row)
}

// RemoveByCode deletes the token by its authorization code.
func (s *TokenStore) RemoveByCode(ctx context.Context, code string) error {
	s.logger.Log(ctx, LogLevelDebug, "removing token by authorization code", "code", code)

	if code == "" {
		s.logger.Log(ctx, LogLevelWarn, "no code was provided")
		return nil
	}

	_, err := s.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE code = $1", s.table), code)

	if !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return err
	}

	s.logger.Log(ctx, LogLevelInfo, "token removed")

	return nil
}

func (s *TokenStore) RemoveByAccess(ctx context.Context, access string) error {
	s.logger.Log(ctx, LogLevelDebug, "removing token by access token", "access", access)

	if access == "" {
		s.logger.Log(ctx, LogLevelWarn, "no access was provided")
		return nil
	}

	_, err := s.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE access_token = $1", s.table), access)

	if !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return err
	}

	s.logger.Log(ctx, LogLevelInfo, "token removed")

	return nil
}

func (s *TokenStore) RemoveByRefresh(ctx context.Context, refresh string) error {
	s.logger.Log(ctx, LogLevelDebug, "removing token by refresh token", "refresh", refresh)

	if refresh == "" {
		s.logger.Log(ctx, LogLevelWarn, "no refresh was provided")
		return nil
	}

	_, err := s.pool.Exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE refresh_token = $1", s.table), refresh)

	if !errors.Is(err, pgx.ErrNoRows) {
		s.logger.Log(ctx, LogLevelError, err.Error())
		return err
	}

	s.logger.Log(ctx, LogLevelInfo, "token removed")

	return nil
}

// Close closes the store and releases any resources.
func (s *TokenStore) Close(ctx context.Context) {
	s.logger.Log(ctx, LogLevelDebug, "closing token store")

	if s.cleanupTicker != nil {
		s.logger.Log(ctx, LogLevelDebug, "stopping cleanup ticker")
		s.cleanupTicker.Stop()
	}

	s.logger.Log(ctx, LogLevelDebug, "token store closed")
}

// NewTokenStore creates a new TokenStore.
func NewTokenStore(opts ...TokenStoreOption) (*TokenStore, error) {
	s := &TokenStore{
		table:  DefaultTokenStoreTable,
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

	s.InitCleanup(context.Background())

	return s, nil
}
