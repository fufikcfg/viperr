package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fufikcfg/viperr"
)

type Config struct {
	App    AppConfig
	DB     DBConfig
	Server ServerConfig
	Logger LoggerConfig
	AppDir string
}

type AppConfig struct {
	Key           string        `config:"APP_KEY"`
	AllowIP       string        `config:"APP_ALLOW_IP"`
	Name          string        `config:"APP_NAME,default=scheduler" validate:"notblank,dns_rfc1035_label"`
	SyncLoop      time.Duration `config:"APP_SYNC_LOOP" validate:"mindur=1s"`
	LockTimeout   time.Duration `config:"APP_LOCK_TIMEOUT,default=5m" validate:"nonneg"`
	WorkerLimit   int           `config:"APP_WORKER_LIMIT" validate:"positive"`
	JobBuffer     int           `config:"APP_JOB_BUFFER" validate:"positive"`
	Timezone      string        `config:"APP_TIMEZONE,default=Europe/Moscow"`
	APIMaxLimit   int           `config:"APP_API_MAX_LIMIT" validate:"nonneg"`
	ShutdownGrace time.Duration `config:"APP_SHUTDOWN_GRACE,default=30s" validate:"nonneg"`
}

type DBConfig struct {
	Host     string        `config:"DB_HOST" validate:"required,notblank,host"`
	Port     string        `config:"DB_PORT,default=5432" validate:"port"`
	User     string        `config:"DB_USER" validate:"required,notblank,max=63"`
	Password string        `config:"DB_PASSWORD" validate:"required"`
	Database string        `config:"DB_NAME" validate:"required,notblank,pg_ident"`
	SSLMode  string        `config:"DB_SSLMODE,default=disable" validate:"pg_sslmode"`
	Timeout  time.Duration `config:"DB_TIMEOUT" validate:"mindur=1s"`
}

type ServerConfig struct {
	Addr            string        `config:"APP_SERVER_URL" validate:"required_without=Port,omitempty,listen_addr"`
	Port            string        `config:"APP_SERVER_PORT" validate:"required_without=Addr,omitempty,port"`
	ShutdownTimeout time.Duration `config:"APP_SERVER_HTTP_TIMEOUT" validate:"mindur=1s"`
}

type LoggerConfig struct {
	Level    string `config:"LOGGER_LEVEL" validate:"required,log_level"`
	Path     string `config:"LOGGER_PATH,path,default=logs" validate:"dirpath"`
	MaxSize  string `config:"LOGGER_MAX_SIZE_MB,default=10"`
	Format   string `config:"LOGGER_FORMAT,default=inline" validate:"oneof_ci=inline console"`
	AppName  string
	MaxBytes int64
}

func Load() (Config, error) {
	dir, err := viperr.ResolveAppDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve app dir: %w", err)
	}

	cfg := Config{AppDir: dir}
	if err := viperr.Load(&cfg, dir); err != nil {
		return Config{}, err
	}

	cfg.Logger.AppName = cfg.App.Name
	return cfg, nil
}

// Normalize runs after binding, before tag validation: it lowercases the
// level and format so log_level and oneof_ci see canonical values,
// and derives MaxBytes from MaxSize.
func (c *LoggerConfig) Normalize() error {
	c.Level = strings.ToLower(strings.TrimSpace(c.Level))
	c.Format = strings.ToLower(strings.TrimSpace(c.Format))

	maxBytes, err := maxSizeBytes(c.MaxSize)
	if err != nil {
		return err
	}
	c.MaxBytes = maxBytes
	return nil
}

func (c ServerConfig) ListenAddr() string {
	if u := strings.TrimSpace(c.Addr); u != "" {
		return u
	}
	return ":" + strings.TrimSpace(c.Port)
}

func (c DBConfig) ConnString() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   "/" + c.Database,
	}
	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// maxSizeBytes parses LOGGER_MAX_SIZE_MB: a bare number is megabytes,
// the K, M, and G suffixes are 1024-based. Invalid values are an error,
// not a silent default.
func maxSizeBytes(raw string) (int64, error) {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	if raw == "" {
		return 10 * 1024 * 1024, nil
	}

	mult := int64(1024 * 1024)
	switch {
	case strings.HasSuffix(raw, "G"):
		mult = 1024 * 1024 * 1024
		raw = strings.TrimSuffix(raw, "G")
	case strings.HasSuffix(raw, "M"):
		raw = strings.TrimSuffix(raw, "M")
	case strings.HasSuffix(raw, "K"):
		mult = 1024
		raw = strings.TrimSuffix(raw, "K")
	}

	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("LOGGER_MAX_SIZE_MB: %w", err)
	}
	if n <= 0 {
		return 0, fmt.Errorf("LOGGER_MAX_SIZE_MB must be > 0, got %d", n)
	}
	return n * mult, nil
}
