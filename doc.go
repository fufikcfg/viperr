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
// options: required, default=VALUE, path, and oneof=A|B.
// An optional source tag pins a field to one named source ("xml", "env", or
// "process"). Without it, Decode searches sources in the order they were passed.
//
//	type Config struct {
//		Key         string        `config:"APP_KEY,required"`
//		LockTimeout time.Duration `config:"APP_LOCK_TIMEOUT,default=5m"`
//		Format      string        `config:"LOGGER_FORMAT,default=inline,oneof=inline|console"`
//		LogPath     string        `config:"LOGGER_PATH,path,default=logs"`
//	}
//
// A path option is resolved against Options.AppDir. Load sets that directory
// from the dir argument, or from ResolveAppDir when dir is empty.
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
package viperr
