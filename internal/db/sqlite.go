package db

import (
	"context"
	"database/sql"
	"net/url"
	"strings"

	// Register the pure-Go SQLite database driver.
	_ "modernc.org/sqlite"

	"github.com/memohai/memoh/internal/config"
)

func OpenSQLite(ctx context.Context, cfg config.SQLiteConfig) (*sql.DB, error) {
	conn, err := sql.Open(DriverSQLite, SQLiteFileDSN(cfg))
	if err != nil {
		return nil, err
	}
	if err := conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func SQLiteFileDSN(cfg config.SQLiteConfig) string {
	if dsn := strings.TrimSpace(cfg.DSN); dsn != "" {
		return sqliteFileDSN(dsn)
	}
	return sqliteFileDSN(SQLiteDSN(cfg))
}

func sqliteFileDSN(dsn string) string {
	path := strings.TrimPrefix(strings.TrimSpace(dsn), "sqlite://")
	queryStart := strings.IndexByte(path, '?')
	if queryStart < 0 {
		return path
	}
	filePath := path[:queryStart]
	rawQuery := path[queryStart+1:]
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return path
	}
	if busyTimeout := query.Get("_busy_timeout"); busyTimeout != "" {
		query.Set("_pragma", "busy_timeout("+busyTimeout+")")
		query.Del("_busy_timeout")
	}
	if query.Get("_journal_mode") == "WAL" {
		query.Add("_pragma", "journal_mode(WAL)")
		query.Del("_journal_mode")
	}
	if encoded := query.Encode(); encoded != "" {
		return filePath + "?" + encoded
	}
	return filePath
}
