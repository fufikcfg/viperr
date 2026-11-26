package viperr

import (
	"github.com/fufikcfg/viperr/internal/resolve"
)

// Resolve joins a configuration path to base.
//
// A native absolute path is cleaned and returned. On Windows a path with a
// drive letter is absolute; a path that only starts with / or \ is relative to
// base. On other systems a Windows path such as C:\logs is treated as relative,
// and the drive prefix is removed.
//
// An empty path stays empty. The caller decides the fallback, usually with default.
func Resolve(path, base string) string {
	return resolve.Resolve(path, base)
}

// ResolveStrict is Resolve with one difference: a path that is absolute in a
// foreign style is returned verbatim instead of being rebased onto base.
// On Windows that is a unix-rooted path (/etc/gateway/routes.d); on other
// systems it is a drive-letter path (C:\var\log). The file then simply is not
// found — an honest error instead of a surprising <base>\etc\... path.
// Fields opt in with the noabs tag option.
func ResolveStrict(path, base string) string {
	return resolve.ResolveStrict(path, base)
}

// ResolveAppDir returns the first of the working directory, the executable
// directory, and the executable parent that contains one of markers.
// With no markers it looks for .env, settings.xml, or settings/settings.xml.
// Pass your own markers (go.mod, routes/) when an application keeps its
// configuration outside those files. If no candidate contains a marker, it
// returns the working directory, then the executable directory.
func ResolveAppDir(markers ...string) (string, error) {
	return resolve.AppDir(markers...)
}
