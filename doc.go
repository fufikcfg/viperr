// Package viperr loads a flat KEY=value configuration from the process
// environment, a .env file, and an XML file whose tag names are keys.
//
// Priority is the order of sources passed to Decode, then the field default.
// Load uses process environment, then .env, then XML. A zero such as "0" is a
// present value and beats a default. An empty XML tag is not present, so it
// does not replace a value from a higher-priority source and does not block a
// default.
//
// Fields carry one config tag. The first element is the key. The rest are
// options separated by commas: default=VALUE, path, noabs, secret, and
// oneof=A|B. A backslash escapes the next character, so `\,` is a literal
// comma inside an option value — a default can hold a list:
//
//	type Config struct {
//		Key         string        `config:"APP_KEY" validate:"required"`
//		LockTimeout time.Duration `config:"APP_LOCK_TIMEOUT,default=5m"`
//		Format      string        `config:"LOGGER_FORMAT,default=inline,oneof=inline|console"`
//		LogPath     string        `config:"LOGGER_PATH,path,default=logs"`
//		RoutesPath  string        `config:"GATEWAY_ROUTES_PATH,path,noabs,default=/etc/gateway/routes.d"`
//		Origins     string        `config:"CORS_ALLOWED_ORIGINS,default=http://localhost:3000\\,http://localhost:8080"`
//		APIKey      string        `config:"GATEWAY_API_KEY,secret"`
//	}
//
// A path option is resolved against Options.AppDir. Load sets that directory
// from the dir argument, or from ResolveAppDir when dir is empty. The noabs
// option keeps a foreign-absolute path (unix-rooted on Windows, drive-letter
// elsewhere) verbatim instead of rebasing it onto AppDir. The secret option
// masks the value in reports; keys that look like credentials are masked even
// without it.
//
// An optional source tag pins a field to one named source ("xml", "env", or
// "process"). Without it, Decode searches sources in the order they were passed.
//
// # One key without a struct
//
// Get reads a single key along the standard chain (process > .env > xml)
// without declaring a struct; GetFrom takes an explicit directory:
//
//	v, ok := viperr.Get("HTTP_PORT")
//	v, ok := viperr.Get("HTTP_PORT", "8080") // default when absent
//
// # Where did a value come from
//
// DecodeReport and LoadReport return a Report — one entry per tagged field
// with the winning source (process / env / xml / default / -) and the value,
// secrets masked. Explain writes that table to any writer:
//
//	report, err := viperr.LoadReport(&cfg, dir)
//	fmt.Print(report)
//	// or in one step:
//	err = viperr.Explain(os.Stdout, &cfg)
//
// # Validation
//
// A second tag, validate, checks the bound value. Commas separate rules;
// | separates alternatives inside one rule; = passes a parameter.
//
//	type Config struct {
//		Host     string        `config:"DB_HOST" validate:"required,notblank,host"`
//		DBPort   string        `config:"DB_PORT,default=5432" validate:"port"`
//		SyncLoop time.Duration `config:"APP_SYNC_LOOP" validate:"mindur=1s"`
//		Workers  int           `config:"APP_WORKER_LIMIT" validate:"positive"`
//		Addr     string        `config:"APP_SERVER_URL" validate:"required_without=Port,omitempty,listen_addr"`
//		Port     string        `config:"APP_SERVER_PORT" validate:"required_without=Addr,omitempty,port"`
//	}
//
// Built-in tags: required, omitempty, required_without, gt, gte, lt, lte,
// min, max, eq, ne (numbers by value, strings by length, durations accept
// "1s" parameters), oneof, port, ip, ipv4, ipv6, fqdn, hostname_rfc1123,
// dns_rfc1035_label, dirpath, duration.
//
// Shipped on top: notblank, mindur, maxdur, listen_addr, oneof_ci,
// writable_dir, pg_ident, and the aliases positive (gt=0), nonneg (gte=0),
// host (hostname_rfc1123|ip|fqdn), log_level, pg_sslmode.
//
// Decode and Load run tag validation automatically, after Normalizer hooks
// and before the Validator hook. WithRules, WithAliases, and WithStructRules
// extend the ruleset; Validate applies the same rules to any struct.
//
// # Layout
//
// The public API lives in this package. The implementation is split into
// internal packages (internal/bind, internal/tag, internal/source,
// internal/resolve, internal/dotenv, internal/xmlconf, internal/validate),
// which the Go toolchain makes importable only by code inside this module.
package viperr
