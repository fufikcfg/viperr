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
// After a successful bind, Decode calls Validate if the destination implements
// Validator. That is the place for rules that are not key presence or a value
// from a fixed set, such as "worker limit must be positive".
package viperr
