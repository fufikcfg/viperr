package viperr

import (
	"github.com/fufikcfg/viperr/internal/validate"
)

// FieldLevel gives a validation rule access to the field being checked.
type FieldLevel = validate.FieldLevel

// Func reports whether a field value satisfies a rule.
type Func = validate.Func

// Rule adds a validation rule or replaces a shipped one (notblank, mindur,
// maxdur, listen_addr, oneof_ci, writable_dir, pg_ident). Replacing a
// built-in tag (required, gt, port, ...) needs WithBuiltinOverride.
type Rule = validate.Rule

// StructRule runs Fn for every struct of the Sample type reached during
// validation, including the root. Fn receives a pointer to the struct when it
// is addressable, a copy otherwise.
type StructRule = validate.StructRule

// Option customizes tag validation: pass to Validate, Load,
// or Options.Validation of Decode.
type Option = validate.Option

// FieldError is one failed rule on one field.
type FieldError = validate.FieldError

// Errors is every rule failure found by one Validate call.
type Errors = validate.Errors

// WithRules adds rules. A duplicate tag inside one call is an error.
// A rule named after a shipped rule replaces it; a rule named after a
// built-in tag is an error unless WithBuiltinOverride is set.
func WithRules(rules ...Rule) Option {
	return validate.WithRules(rules...)
}

// WithAliases adds tag aliases. An alias expands inline where its name
// appears: "positive" -> "gt=0", "host" -> "hostname_rfc1123|ip|fqdn".
// An alias named after a shipped alias replaces it; an alias named after a
// built-in tag is an error unless WithBuiltinOverride is set.
func WithAliases(aliases map[string]string) Option {
	return validate.WithAliases(aliases)
}

// WithStructRules adds struct-level checks, for rules a single field
// cannot express.
func WithStructRules(rules ...StructRule) Option {
	return validate.WithStructRules(rules...)
}

// WithBuiltinOverride allows WithRules and WithAliases to replace built-in
// tags such as gt or port. It changes the meaning of those tags for every
// struct validated with the same options.
func WithBuiltinOverride() Option {
	return validate.WithBuiltinOverride()
}

// Validate checks dst against its validate tags. dst must be a pointer to a
// struct; nested structs are validated recursively. A field named in an error
// is its config key (APP_SYNC_LOOP), or its Go name when untagged.
//
//	validate:"required,notblank"   non-empty, non-whitespace string
//	validate:"positive"            alias for gt=0
//	validate:"mindur=1s"           time.Duration >= 1s
//	validate:"host"                alias for hostname_rfc1123|ip|fqdn
//	validate:"omitempty,port"      empty skips the rest
//	validate:"required_without=Port,omitempty,listen_addr"
//
// Directives come first: required, required_without, omitempty.
func Validate(dst any, opts ...Option) error {
	return validate.Validate(dst, opts...)
}
