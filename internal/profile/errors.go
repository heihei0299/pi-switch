package profile

import (
	"errors"
	"fmt"

	domainprovider "github.com/heihei0299/pi-switch/internal/domain/provider"
)

// Profile-domain outcomes callers must tell apart: the HTTP handlers map them to
// status codes and the CLI to exit codes. They exist because classification by
// message text cannot work — a persist failure whose path happened to contain
// "not found" was answered 404 instead of 500.
var (
	ErrProfileNotFound     = errors.New("profile not found")
	ErrProfileExists       = errors.New("profile already exists")
	ErrUnknownChannel      = domainprovider.ErrUnknownChannel
	ErrUpstreamFetchFailed = errors.New("upstream fetch failed")
)

// profileError carries one of the kinds above together with the message its
// surface should print. Keeping message and kind apart is what let the kinds be
// introduced without touching a single response body: every caller still prints
// err.Error() exactly as before.
type profileError struct {
	kind error
	msg  string
}

func (e profileError) Error() string { return e.msg }
func (e profileError) Unwrap() error { return e.kind }

func profileErr(kind error, format string, args ...any) error {
	return profileError{kind: kind, msg: fmt.Sprintf(format, args...)}
}
