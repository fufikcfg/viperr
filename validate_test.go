package viperr

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateBuiltins(t *testing.T) {
	type section struct {
		Name     string        `config:"APP_NAME" validate:"required,notblank,dns_rfc1035_label"`
		User     string        `config:"DB_USER" validate:"required,notblank,max=63"`
		Port     string        `config:"DB_PORT" validate:"port"`
		Host     string        `config:"DB_HOST" validate:"required,host"`
		SyncLoop time.Duration `config:"APP_SYNC_LOOP" validate:"mindur=1s"`
		Workers  int           `config:"APP_WORKER_LIMIT" validate:"positive"`
		Limit    int           `config:"APP_API_MAX_LIMIT" validate:"nonneg"`
		Level    string        `config:"LOGGER_LEVEL" validate:"required,log_level"`
		SSLMode  string        `config:"DB_SSLMODE" validate:"pg_sslmode"`
	}
	cfg := section{
		Name: "scheduler", User: "root", Port: "5432", Host: "db.local",
		SyncLoop: time.Second, Workers: 4, Limit: 0, Level: "info", SSLMode: "require",
	}
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}

	bad := section{} // everything zero
	err := Validate(&bad)
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	for _, key := range []string{"APP_NAME", "DB_USER", "DB_PORT", "DB_HOST", "APP_SYNC_LOOP", "APP_WORKER_LIMIT", "LOGGER_LEVEL", "DB_SSLMODE"} {
		if !strings.Contains(msg, key) {
			t.Fatalf("missing %s in: %s", key, msg)
		}
	}
	// nonneg: 0 is allowed
	if strings.Contains(msg, "APP_API_MAX_LIMIT") {
		t.Fatalf("zero nonneg must pass: %s", msg)
	}
}

func TestValidateGtGteOnStringsAndDurations(t *testing.T) {
	var cfg struct {
		User    string        `config:"DB_USER" validate:"max=5"`
		Timeout time.Duration `config:"DB_TIMEOUT" validate:"gt=500ms"`
	}
	cfg.User = "too-long-user"
	cfg.Timeout = 100 * time.Millisecond
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "DB_USER length must be <= 5") {
		t.Fatal(msg)
	}
	if !strings.Contains(msg, "DB_TIMEOUT must be > 500ms") {
		t.Fatal(msg)
	}
	cfg.User = "short"
	cfg.Timeout = time.Second
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRequiredWithout(t *testing.T) {
	type server struct {
		Addr string `config:"APP_SERVER_URL" validate:"required_without=Port,omitempty,listen_addr"`
		Port string `config:"APP_SERVER_PORT" validate:"required_without=Addr,omitempty,port"`
	}
	var cfg server
	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "APP_SERVER_URL is required when Port is not set") {
		t.Fatalf("got %v", err)
	}
	cfg.Addr = ":8080"
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Addr = "not-an-addr"
	if err := Validate(&cfg); err == nil || !strings.Contains(err.Error(), "[host]:port") {
		t.Fatalf("got %v", err)
	}
	cfg.Addr = ""
	cfg.Port = "8080"
	if err := Validate(&cfg); err != nil {
		t.Fatal(err)
	}
}

func TestValidateOneofCIAndCustomRules(t *testing.T) {
	var cfg struct {
		Format string `config:"LOGGER_FORMAT" validate:"oneof_ci=inline console"`
		TZ     string `config:"APP_TIMEZONE" validate:"cron_tz"`
	}
	cfg.Format = "INLINE"
	cfg.TZ = "Europe/Moscow"
	err := Validate(&cfg,
		WithRules(Rule{
			Tag: "cron_tz",
			Fn: func(fl FieldLevel) bool {
				_, err := time.LoadLocation(fl.Field().String())
				return err == nil
			},
			Message: "{field} must be a valid IANA timezone",
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	cfg.TZ = "Nowhere/Special"
	err = Validate(&cfg,
		WithRules(Rule{
			Tag:     "cron_tz",
			Fn:      func(fl FieldLevel) bool { return false },
			Message: "{field} must be a valid IANA timezone",
		}),
	)
	if err == nil || !strings.Contains(err.Error(), "APP_TIMEZONE must be a valid IANA timezone") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateCollisionPolicy(t *testing.T) {
	var cfg struct {
		N int `config:"APP_N" validate:"positive"`
	}
	rule := Rule{Tag: "gt", Fn: func(fl FieldLevel) bool { return true }}
	if err := Validate(&cfg, WithRules(rule)); err == nil || !strings.Contains(err.Error(), "shadows a built-in tag") {
		t.Fatalf("builtin override must fail: %v", err)
	}
	if err := Validate(&cfg, WithRules(rule), WithBuiltinOverride()); err != nil {
		t.Fatalf("WithBuiltinOverride must allow: %v", err)
	}
	dup := Rule{Tag: "x", Fn: func(fl FieldLevel) bool { return true }}
	err := Validate(&cfg, WithRules(dup, dup))
	if err == nil || !strings.Contains(err.Error(), "duplicate rule") {
		t.Fatalf("duplicate must fail: %v", err)
	}
	// user rule replaces a shipped rule
	nb := Rule{Tag: "notblank", Fn: func(fl FieldLevel) bool { return true }, Message: "custom"}
	var s struct {
		V string `config:"APP_V" validate:"notblank"`
	}
	if err := Validate(&s, WithRules(nb)); err != nil {
		t.Fatalf("shipped rule replacement must pass: %v", err)
	}
	if err := Validate(&cfg, WithAliases(map[string]string{"gt": "lt=0"})); err == nil {
		t.Fatal("alias shadowing a builtin must fail")
	}
}

func TestValidateStructRuleAndSkip(t *testing.T) {
	type rangeCfg struct {
		Min  int    `config:"APP_MIN"`
		Max  int    `config:"APP_MAX"`
		Skip string `validate:"-"`
	}
	var cfg rangeCfg
	cfg.Min, cfg.Max = 10, 5
	sr := StructRule{
		Sample: rangeCfg{},
		Fn: func(v any) error {
			r := v.(*rangeCfg)
			if r.Min > r.Max {
				return errString("APP_MIN must not exceed APP_MAX")
			}
			return nil
		},
	}
	err := Validate(&cfg, WithStructRules(sr))
	if err == nil || !strings.Contains(err.Error(), "APP_MIN must not exceed APP_MAX") {
		t.Fatalf("got %v", err)
	}
	cfg.Max = 20
	if err := Validate(&cfg, WithStructRules(sr)); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeRunsTagValidation(t *testing.T) {
	var cfg struct {
		Workers int    `config:"APP_WORKER_LIMIT" validate:"positive"`
		Host    string `config:"DB_HOST" validate:"required,notblank"`
	}
	err := Decode(&cfg, Options{}, Map{"APP_WORKER_LIMIT": "0", "DB_HOST": "  "})
	if err == nil {
		t.Fatal("expected validation errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "APP_WORKER_LIMIT must be > 0") {
		t.Fatal(msg)
	}
	if !strings.Contains(msg, "DB_HOST must not be blank") {
		t.Fatal(msg)
	}
	var ferrs Errors
	if !errors.As(err, &ferrs) || len(ferrs) != 2 {
		t.Fatalf("want Errors with 2 entries, got %v", err)
	}
}

func TestDecodeNormalizerBeforeValidation(t *testing.T) {
	var cfg loggerSection
	err := Decode(&cfg, Options{}, Map{"LOGGER_LEVEL": "INFO", "LOGGER_FORMAT": "Console"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Level != "info" || cfg.Format != "console" {
		t.Fatalf("normalize did not run: %+v", cfg)
	}
}

type loggerSection struct {
	Level  string `config:"LOGGER_LEVEL" validate:"required,log_level"`
	Format string `config:"LOGGER_FORMAT" validate:"oneof_ci=inline console"`
}

func (l *loggerSection) Normalize() error {
	l.Level = strings.ToLower(strings.TrimSpace(l.Level))
	l.Format = strings.ToLower(strings.TrimSpace(l.Format))
	return nil
}

func TestValidateNilPointer(t *testing.T) {
	var cfg struct {
		Port *int `config:"APP_PORT" validate:"required"`
	}
	err := Validate(&cfg)
	if err == nil || !strings.Contains(err.Error(), "APP_PORT is required") {
		t.Fatalf("nil pointer must fail required: %v", err)
	}
	var opt struct {
		Port *int `config:"APP_PORT" validate:"omitempty,port"`
	}
	if err := Validate(&opt); err != nil {
		t.Fatalf("nil pointer with omitempty must pass: %v", err)
	}
}

func TestValidateFieldKeyFallback(t *testing.T) {
	var cfg struct {
		Untagged int `validate:"positive"`
	}
	if err := Validate(&cfg); err == nil || !strings.Contains(err.Error(), "Untagged must be > 0") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadWithValidationOptions(t *testing.T) {
	var cfg struct {
		TZ string `config:"APP_TIMEZONE" validate:"tz"`
	}
	err := Decode(&cfg, Options{
		Validation: []Option{WithRules(Rule{
			Tag:     "tz",
			Fn:      func(fl FieldLevel) bool { _, err := time.LoadLocation(fl.Field().String()); return err == nil },
			Message: "{field} must be a valid IANA timezone",
		})},
	}, Map{"APP_TIMEZONE": "Mars/Olympus"})
	if err == nil || !strings.Contains(err.Error(), "APP_TIMEZONE must be a valid IANA timezone") {
		t.Fatalf("got %v", err)
	}
}
