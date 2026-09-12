package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	"lms-website-be/internal/config"

	"github.com/go-sql-driver/mysql"
)

// Open creates a MySQL connection pool and verifies that the server is reachable.
func Open(cfg config.Config) (*sql.DB, error) {
	mysqlConfig := mysql.NewConfig()
	mysqlConfig.User, mysqlConfig.Passwd = cfg.DBUser, cfg.DBPassword
	mysqlConfig.Net, mysqlConfig.Addr = "tcp", net.JoinHostPort(cfg.DBHost, cfg.DBPort)
	mysqlConfig.DBName, mysqlConfig.ParseTime = cfg.DBName, true
	mysqlConfig.Loc = time.UTC
	mysqlConfig.Timeout = 5 * time.Second
	mysqlConfig.ReadTimeout, mysqlConfig.WriteTimeout = 15*time.Second, 15*time.Second
	mysqlConfig.Collation = "utf8mb4_unicode_ci"
	mysqlConfig.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}
	dsn := mysqlConfig.FormatDSN()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("membuka koneksi database: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("database tidak dapat dihubungi: %w", err)
	}

	return db, nil
}
