package database

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	defaultDatabase      = "refratia"
	defaultAdminDatabase = "postgres"
	defaultPort          = 5432
)

var databaseNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Config contains the connection settings for the RefratIA PostgreSQL database.
// DATABASE_URL takes precedence over the individual POSTGRES_* variables.
type Config struct {
	URL           string
	Host          string
	Port          int
	User          string
	Password      string
	Name          string
	SSLMode       string
	AdminDatabase string
}

// ConfigFromEnv loads the same environment contract used by wallet-analysis.
func ConfigFromEnv() Config {
	loadDotEnv()
	return Config{
		URL:           os.Getenv("DATABASE_URL"),
		Host:          os.Getenv("POSTGRES_HOST"),
		Port:          envInt("POSTGRES_PORT", defaultPort),
		User:          os.Getenv("POSTGRES_USER"),
		Password:      os.Getenv("POSTGRES_PASSWORD"),
		Name:          envOrDefault("POSTGRES_DB", defaultDatabase),
		SSLMode:       envOrDefault("POSTGRES_SSLMODE", "disable"),
		AdminDatabase: envOrDefault("POSTGRES_ADMIN_DB", defaultAdminDatabase),
	}
}

// DB is the small persistence seam used by the backend. Its Query and Exec
// methods intentionally expose pgx primitives so feature code can add tables
// and CRUD operations without another abstraction layer.
type DB struct {
	Gorm *gorm.DB
}

// Open ensures the configured database exists, then opens and pings a pool.
func Open(ctx context.Context) (*DB, error) {
	config := ConfigFromEnv()
	if err := EnsureDatabase(ctx, config); err != nil {
		return nil, err
	}

	gormDB, err := gorm.Open(postgres.Open(config.dsn()), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("create postgres ORM connection: %w", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("get postgres SQL connection: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{Gorm: gormDB}, nil
}

// EnsureDatabase creates the configured database when it does not exist.
// The PostgreSQL user must be allowed to connect to the admin database and
// create databases (usually CREATEDB or an administrative role).
func EnsureDatabase(ctx context.Context, config Config) error {
	if config.Name == "" {
		config.Name = defaultDatabase
	}
	if !databaseNamePattern.MatchString(config.Name) {
		return fmt.Errorf("invalid postgres database name %q", config.Name)
	}

	adminDSN, err := config.adminDSN()
	if err != nil {
		return err
	}
	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("connect to postgres admin database: %w", err)
	}
	defer adminPool.Close()

	if err := adminPool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres admin database: %w", err)
	}

	var exists bool
	if err := adminPool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)",
		config.Name,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check postgres database: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := adminPool.Exec(ctx, "CREATE DATABASE "+quoteIdentifier(config.Name)); err != nil {
		// Another process may have created it between the existence check and
		// CREATE DATABASE. Confirm that case before returning the original error.
		if !databaseExists(ctx, adminPool, config.Name) {
			return fmt.Errorf("create postgres database %q: %w", config.Name, err)
		}
	}
	return nil
}

func (db *DB) Close() {
	if db != nil && db.Gorm != nil {
		if sqlDB, err := db.Gorm.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (db *DB) Exec(sql string, args ...any) *gorm.DB {
	return db.Gorm.Exec(sql, args...)
}

func (db *DB) AutoMigrate(models ...any) error {
	return db.Gorm.AutoMigrate(models...)
}

func (config Config) dsn() string {
	if config.URL != "" {
		return config.URL
	}
	return config.dsnForDatabase(config.Name)
}

func (config Config) adminDSN() (string, error) {
	if config.AdminDatabase == "" {
		config.AdminDatabase = defaultAdminDatabase
	}
	if config.URL == "" {
		return config.dsnForDatabase(config.AdminDatabase), nil
	}

	parsed, err := url.Parse(config.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid DATABASE_URL")
	}
	parsed.Path = "/" + config.AdminDatabase
	return parsed.String(), nil
}

func (config Config) dsnForDatabase(database string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(config.User),
		url.QueryEscape(config.Password),
		config.Host,
		config.Port,
		database,
		url.QueryEscape(config.SSLMode),
	)
}

func databaseExists(ctx context.Context, pool *pgxpool.Pool, name string) bool {
	var exists bool
	return pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)",
		name,
	).Scan(&exists) == nil && exists
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv() {
	for _, path := range []string{".env", "../.env", "backend/.env"} {
		loadDotEnvFile(path)
	}
}

func loadDotEnvFile(path string) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return
	}

	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || os.Getenv(name) != "" {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		_ = os.Setenv(name, value)
	}
}
