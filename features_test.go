package viperr

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGetSingleKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("HTTP_PORT=9090\nGATEWAY_API_KEY=sekret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.xml"), []byte(`<root><HTTP_PORT>7070</HTTP_PORT><XML_ONLY>x</XML_ONLY></root>`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_API_KEY", "from-proc")

	// .env beats xml.
	if v, ok := GetFrom(dir, "HTTP_PORT"); !ok || v != "9090" {
		t.Fatalf("HTTP_PORT: got %q ok=%v", v, ok)
	}
	// process beats .env.
	if v, ok := GetFrom(dir, "GATEWAY_API_KEY"); !ok || v != "from-proc" {
		t.Fatalf("GATEWAY_API_KEY: got %q ok=%v", v, ok)
	}
	// xml is still in the chain.
	if v, ok := GetFrom(dir, "XML_ONLY"); !ok || v != "x" {
		t.Fatalf("XML_ONLY: got %q ok=%v", v, ok)
	}
	// default when absent, ok=false.
	if v, ok := GetFrom(dir, "MISSING", "8080"); ok || v != "8080" {
		t.Fatalf("default: got %q ok=%v", v, ok)
	}
	// absent without default.
	if v, ok := GetFrom(dir, "MISSING"); ok || v != "" {
		t.Fatalf("absent: got %q ok=%v", v, ok)
	}
}

func TestGet(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("LONE_KEY=found\n"), 0o644); err != nil {
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
	if v, ok := Get("LONE_KEY"); !ok || v != "found" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
	if v, ok := Get("NOPE", "dflt"); ok || v != "dflt" {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}

func TestDefaultWithEscapedComma(t *testing.T) {
	var cfg struct {
		Origins string `config:"CORS_ALLOWED_ORIGINS,default=http://localhost:3000\\,http://localhost:8080"`
	}
	if err := Decode(&cfg, Options{}, Map{}); err != nil {
		t.Fatal(err)
	}
	want := "http://localhost:3000,http://localhost:8080"
	if cfg.Origins != want {
		t.Fatalf("got %q want %q", cfg.Origins, want)
	}
}

func TestDefaultWithEscapedBackslash(t *testing.T) {
	var cfg struct {
		Dir string `config:"APP_DIR,default=C:\\\\opt\\\\app"`
	}
	if err := Decode(&cfg, Options{}, Map{}); err != nil {
		t.Fatal(err)
	}
	if cfg.Dir != `C:\opt\app` {
		t.Fatalf("got %q", cfg.Dir)
	}
}

func TestNoAbsKeepsForeignAbsolute(t *testing.T) {
	var cfg struct {
		Strict string `config:"GATEWAY_ROUTES_PATH,path,noabs,default=/etc/gateway/routes.d"`
		Plain  string `config:"APP_LOGS_PATH,path,default=/etc/app/logs"`
	}
	err := Decode(&cfg, Options{AppDir: filepath.Join("app", "dir")}, Map{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if cfg.Strict != "/etc/gateway/routes.d" {
			t.Fatalf("noabs rebased foreign path: %q", cfg.Strict)
		}
		if cfg.Plain == "/etc/app/logs" {
			t.Fatalf("path without noabs should be rebased, got %q", cfg.Plain)
		}
	} else {
		if cfg.Strict != "/etc/gateway/routes.d" || cfg.Plain != "/etc/app/logs" {
			t.Fatalf("native absolute must stay absolute: %q %q", cfg.Strict, cfg.Plain)
		}
	}
}

func TestResolveStrictWindowsPathOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("foreign drive-letter path is native on Windows")
	}
	got := ResolveStrict(`C:\var\log`, "base")
	if got != `C:\var\log` {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeReportProvenance(t *testing.T) {
	proc := Name(SourceProcess, Map{"APP_KEY": "from-proc"})
	env := Name(SourceEnv, Map{"APP_KEY": "from-env", "DB_HOST": "from-env"})

	var cfg struct {
		Key     string `config:"APP_KEY"`
		Host    string `config:"DB_HOST"`
		Workers int    `config:"APP_WORKER_LIMIT,default=4"`
		Missing string `config:"APP_MISSING"`
	}
	report, err := DecodeReport(&cfg, Options{}, proc, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 4 {
		t.Fatalf("entries: %d", len(report))
	}
	byKey := map[string]Entry{}
	for _, e := range report {
		byKey[e.Key] = e
	}
	if e := byKey["APP_KEY"]; e.Source != SourceProcess || e.Value != "***" || !e.Secret {
		t.Fatalf("APP_KEY entry: %+v (process value, masked as *_KEY)", e)
	}
	if e := byKey["DB_HOST"]; e.Source != SourceEnv || e.Value != "from-env" || e.Secret {
		t.Fatalf("DB_HOST entry: %+v", e)
	}
	if e := byKey["APP_WORKER_LIMIT"]; e.Source != "default" || e.Value != "4" {
		t.Fatalf("APP_WORKER_LIMIT entry: %+v", e)
	}
	if e := byKey["APP_MISSING"]; e.Source != "-" || e.Value != "" {
		t.Fatalf("APP_MISSING entry: %+v", e)
	}
}

func TestReportMasksSecrets(t *testing.T) {
	var cfg struct {
		APIKey  string `config:"GATEWAY_API_KEY"`
		Token   string `config:"AUTH_TOKEN"`
		Marked  string `config:"PLAINTEXT,secret"`
		Visible string `config:"LOGGER_FORMAT"`
	}
	report, err := DecodeReport(&cfg, Options{}, Map{
		"GATEWAY_API_KEY": "k1",
		"AUTH_TOKEN":      "t2",
		"PLAINTEXT":       "p3",
		"LOGGER_FORMAT":   "inline",
	})
	if err != nil {
		t.Fatal(err)
	}
	table := report.String()
	for _, leaked := range []string{"k1", "t2", "p3"} {
		if strings.Contains(table, leaked) {
			t.Fatalf("secret %q leaked into report:\n%s", leaked, table)
		}
	}
	if !strings.Contains(table, "inline") {
		t.Fatalf("non-secret value missing:\n%s", table)
	}
	if !strings.Contains(table, "***") {
		t.Fatalf("mask missing:\n%s", table)
	}
}

func TestExplain(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_KEY=k\nDB_HOST=db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Key  string `config:"APP_KEY"`
		Host string `config:"DB_HOST"`
	}
	var out strings.Builder
	if err := Explain(&out, &cfg, dir); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "APP_KEY") || !strings.Contains(text, SourceEnv) || !strings.Contains(text, "DB_HOST") {
		t.Fatalf("table incomplete:\n%s", text)
	}
	if strings.Contains(text, ">k<") || strings.Contains(text, "\tk\t") {
		t.Fatalf("APP_KEY value must be masked:\n%s", text)
	}
}

func TestResolveAppDirCustomMarkers(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
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
	got, err := ResolveAppDir("go.mod")
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

func TestNestedFieldNamesInReport(t *testing.T) {
	var cfg struct {
		DB struct {
			Host string `config:"DB_HOST"`
		}
	}
	report, err := DecodeReport(&cfg, Options{}, Map{"DB_HOST": "db"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 || report[0].Field != "DB.Host" {
		t.Fatalf("%+v", report)
	}
}
