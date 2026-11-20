package viperr

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type appConfig struct {
	Key         string        `config:"APP_KEY"`
	LockTimeout time.Duration `config:"APP_LOCK_TIMEOUT,default=5m"`
	Workers     int           `config:"APP_WORKER_LIMIT,default=4"`
	Format      string        `config:"LOGGER_FORMAT,default=inline,oneof=inline|console"`
	LogPath     string        `config:"LOGGER_PATH,path,default=logs"`
	Enabled     bool          `config:"APP_ENABLED,default=true"`
	Port        *int          `config:"APP_PORT"`
	Nested      struct {
		Host string `config:"DB_HOST"`
	}
	Skip string `config:"-"`
}

func (c appConfig) Validate() error {
	if c.Workers < 1 {
		return errWorkers
	}
	return nil
}

var errWorkers = errString("worker limit must be positive")

type errString string

func (e errString) Error() string { return string(e) }

func TestDecodePriorityAndZero(t *testing.T) {
	xmlSrc := Map{
		"APP_KEY":          "",
		"APP_LOCK_TIMEOUT": "0",
		"DB_HOST":          "from-xml",
		"LOGGER_PATH":      "from-xml",
	}
	delete(xmlSrc, "APP_KEY")
	envSrc := Map{
		"APP_KEY":     "from-env",
		"LOGGER_PATH": "from-env",
		"DB_HOST":     "",
	}
	proc := Map{
		"APP_KEY": "from-proc",
	}

	var cfg appConfig
	err := Decode(&cfg, Options{AppDir: filepath.Join("app", "dir")}, proc, envSrc, xmlSrc)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "from-proc" {
		t.Fatalf("key: got %q", cfg.Key)
	}
	if cfg.LockTimeout != 0 {
		t.Fatalf("zero duration lost to default: %s", cfg.LockTimeout)
	}
	if cfg.Nested.Host != "" {
		t.Fatalf("empty .env value must block xml, got %q", cfg.Nested.Host)
	}
	if cfg.Workers != 4 || cfg.Format != "inline" || !cfg.Enabled {
		t.Fatalf("defaults: workers=%d format=%q enabled=%v", cfg.Workers, cfg.Format, cfg.Enabled)
	}
	wantPath := filepath.Join("app", "dir", "from-env")
	if cfg.LogPath != wantPath {
		t.Fatalf("path: got %q want %q", cfg.LogPath, wantPath)
	}
	if cfg.Port != nil {
		t.Fatal("missing optional pointer should stay nil")
	}
}

func TestEmptyXMLDoesNotBlockDefault(t *testing.T) {
	xmlText := `<settings><app><APP_LOCK_TIMEOUT></APP_LOCK_TIMEOUT><APP_KEY>abc</APP_KEY></app><db><DB_HOST>localhost</DB_HOST></db></settings>`
	src, err := ParseXML(strings.NewReader(xmlText))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := src.Lookup("APP_LOCK_TIMEOUT"); ok {
		t.Fatal("empty tag counted as present")
	}
	var cfg struct {
		Key     string        `config:"APP_KEY"`
		Timeout time.Duration `config:"APP_LOCK_TIMEOUT,default=5m"`
		Host    string        `config:"DB_HOST"`
	}
	if err := Decode(&cfg, Options{}, src); err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout != 5*time.Minute || cfg.Key != "abc" || cfg.Host != "localhost" {
		t.Fatalf("%+v", cfg)
	}
}

func TestRequiredMissing(t *testing.T) {
	var cfg struct {
		Key  string `config:"APP_KEY" validate:"required"`
		Host string `config:"DB_HOST" validate:"required"`
	}
	err := Decode(&cfg, Options{}, Map{})
	if err == nil {
		t.Fatal("expected required errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "APP_KEY") || !strings.Contains(msg, "DB_HOST") {
		t.Fatal(msg)
	}
}

func TestAbsentKeyWithoutDefaultStaysZero(t *testing.T) {
	var cfg struct {
		Key string `config:"APP_KEY"`
		N   int    `config:"APP_N"`
	}
	if err := Decode(&cfg, Options{}, Map{}); err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "" || cfg.N != 0 {
		t.Fatalf("%+v", cfg)
	}
}

func TestDefaultWhenKeyMissing(t *testing.T) {
	var cfg struct {
		Key string `config:"APP_KEY,default=dev"`
	}
	if err := Decode(&cfg, Options{}, Map{}); err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "dev" {
		t.Fatal(cfg.Key)
	}
}

func TestOneofAndParseErrors(t *testing.T) {
	var cfg struct {
		Format string `config:"LOGGER_FORMAT,oneof=inline|console"`
		N      int    `config:"APP_WORKER_LIMIT"`
	}
	err := Decode(&cfg, Options{}, Map{"LOGGER_FORMAT": "json", "APP_WORKER_LIMIT": "nope"})
	if err == nil {
		t.Fatal("expected errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "LOGGER_FORMAT") || !strings.Contains(msg, "APP_WORKER_LIMIT") {
		t.Fatal(msg)
	}
}

func TestValidateHook(t *testing.T) {
	var cfg appConfig
	err := Decode(&cfg, Options{}, Map{
		"APP_KEY":          "k",
		"DB_HOST":          "h",
		"APP_WORKER_LIMIT": "0",
	})
	if err == nil || !strings.Contains(err.Error(), "worker limit") {
		t.Fatalf("got %v", err)
	}
}

func TestEnvParser(t *testing.T) {
	text := "\n# comment\n APP_KEY = secret \nEMPTY=\nAPP_KEY=second\n"
	m, err := ParseEnv(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := m.Lookup("APP_KEY"); v != "second" {
		t.Fatal(v)
	}
	if v, ok := m.Lookup("EMPTY"); !ok || v != "" {
		t.Fatalf("empty present=%v value=%q", ok, v)
	}
	if _, err := ParseEnv(strings.NewReader("NOT_A_PAIR\n")); err == nil {
		t.Fatal("expected line error")
	}
}

func TestLoadFilesAndProcessEnv(t *testing.T) {
	dir := t.TempDir()
	envBody := "APP_KEY=from-env\nAPP_LOCK_TIMEOUT=1s\nLOGGER_PATH=env-logs\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envBody), 0o644); err != nil {
		t.Fatal(err)
	}
	xmlBody := `<root><APP_KEY>from-xml</APP_KEY><APP_LOCK_TIMEOUT>9s</APP_LOCK_TIMEOUT><DB_HOST>db</DB_HOST><LOGGER_PATH>xml-logs</LOGGER_PATH></root>`
	if err := os.WriteFile(filepath.Join(dir, "settings.xml"), []byte(xmlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_KEY", "from-proc")

	var cfg appConfig
	if err := Load(&cfg, dir); err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "from-proc" {
		t.Fatalf("process env lost: %q", cfg.Key)
	}
	if cfg.LockTimeout != time.Second {
		t.Fatalf("env should beat xml, got %s", cfg.LockTimeout)
	}
	if cfg.Nested.Host != "db" {
		t.Fatal(cfg.Nested.Host)
	}
	want := filepath.Join(dir, "env-logs")
	if cfg.LogPath != want {
		t.Fatalf("path: got %q want %q", cfg.LogPath, want)
	}
}

func TestLoadNestedSettings(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "settings")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `<root><APP_KEY>k</APP_KEY><DB_HOST>h</DB_HOST></root>`
	if err := os.WriteFile(filepath.Join(nested, "settings.xml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var cfg appConfig
	if err := Load(&cfg, dir); err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "k" || cfg.Nested.Host != "h" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.LogPath != filepath.Join(dir, "logs") {
		t.Fatal(cfg.LogPath)
	}
}

func TestResolve(t *testing.T) {
	base := filepath.Join("root", "app")
	if got := Resolve("  ", base); got != "" {
		t.Fatal(got)
	}
	if got := Resolve("logs", base); got != filepath.Join(base, "logs") {
		t.Fatal(got)
	}
	if got := Resolve("", base); got != "" {
		t.Fatal(got)
	}

	if runtime.GOOS == "windows" {
		if got := Resolve(`C:\var\log`, base); got != filepath.Clean(`C:\var\log`) {
			t.Fatal(got)
		}
		if got := Resolve(`/logs`, base); got != filepath.Join(base, "logs") {
			t.Fatal(got)
		}
		return
	}
	if got := Resolve("/var/log", base); got != "/var/log" {
		t.Fatal(got)
	}
	if got := Resolve(`C:\var\log`, base); got != filepath.Join(base, "var", "log") {
		t.Fatal(got)
	}
}

func TestResolveAppDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveAppDir()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err = filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestSourcePin(t *testing.T) {
	proc := Name(SourceProcess, Map{"APP_KEY": "from-proc", "DB_HOST": "from-proc"})
	env := Name(SourceEnv, Map{"APP_KEY": "from-env", "DB_HOST": "from-env", "LOGGER_FORMAT": "console"})
	xml := Name(SourceXML, Map{"APP_KEY": "from-xml", "LOGGER_PATH": "xml-logs"})

	var cfg struct {
		Key     string `config:"APP_KEY"`
		FromXML string `config:"APP_KEY" source:"xml"`
		Host    string `config:"DB_HOST" source:"env"`
		Path    string `config:"LOGGER_PATH,path,default=logs" source:"xml"`
		Format  string `config:"LOGGER_FORMAT,default=inline" source:"xml"`
	}
	err := Decode(&cfg, Options{AppDir: "app"}, proc, env, xml)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "from-proc" || cfg.FromXML != "from-xml" || cfg.Host != "from-env" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Path != filepath.Join("app", "xml-logs") {
		t.Fatal(cfg.Path)
	}
	if cfg.Format != "inline" {
		t.Fatalf("missing xml key must use default, got %q", cfg.Format)
	}
}

func TestSourceMissing(t *testing.T) {
	var cfg struct {
		Key string `config:"APP_KEY" source:"xml"`
	}
	err := Decode(&cfg, Options{}, Map{"APP_KEY": "x"})
	if err == nil || !strings.Contains(err.Error(), "xml") {
		t.Fatal(err)
	}
}

func TestLoadSourceXML(t *testing.T) {
	dir := t.TempDir()
	envBody := "APP_KEY=from-env\nTITLE=from-env\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envBody), 0o644); err != nil {
		t.Fatal(err)
	}
	xmlBody := `<root><APP_KEY>from-xml</APP_KEY><DB_HOST>db</DB_HOST><TITLE>from-xml</TITLE></root>`
	if err := os.WriteFile(filepath.Join(dir, "settings.xml"), []byte(xmlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_KEY", "from-proc")
	t.Setenv("TITLE", "from-proc")

	var cfg struct {
		Key   string `config:"APP_KEY"`
		Title string `config:"TITLE" source:"xml"`
		Host  string `config:"DB_HOST"`
	}
	if err := Load(&cfg, dir); err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "from-proc" || cfg.Title != "from-xml" || cfg.Host != "db" {
		t.Fatalf("%+v", cfg)
	}
}

func TestBadTag(t *testing.T) {
	var cfg struct {
		Key string `config:"APP_KEY,nope"`
	}
	err := Decode(&cfg, Options{}, Map{"APP_KEY": "x"})
	if err == nil || !strings.Contains(err.Error(), "unknown option") {
		t.Fatal(err)
	}
}
