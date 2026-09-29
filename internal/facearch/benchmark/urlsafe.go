package benchmark

import (
	"errors"
	"net/url"
	"regexp"
)

// UnparseableURL stands in for a URL SafeURL cannot parse: nothing of it is
// printed, since its parts cannot be told apart.
const UnparseableURL = "(unparseable URL)"

// SafeURL renders raw as scheme://host/path only: the userinfo (user and
// password), the query and the fragment are dropped, so a credential or
// token carried in any of them is never printed. It is the one sanitiser
// for every URL the harness prints or returns in an error.
func SafeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return UnparseableURL
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: u.Path, RawPath: u.RawPath}).String()
}

// urlInText matches an absolute URL inside an error message, up to the
// next space or quote (net/http quotes the URL in a *url.Error).
var urlInText = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.\-]*://[^\s"']+`)

// safeError is err with every URL in its message passed through SafeURL.
// It unwraps to the sentinels callers test for and to a copy of any
// *url.Error whose URL is sanitised, never to the original error, whose
// text may still carry the secrets.
type safeError struct {
	msg  string
	wrap []error
}

func (e *safeError) Error() string   { return e.msg }
func (e *safeError) Unwrap() []error { return e.wrap }

// sanitizeError rewrites err so neither its message nor anything it
// unwraps to carries a URL's userinfo, query or fragment. A *url.Error
// found with errors.As is copied with its URL through SafeURL. The message
// is rewritten as text as well, because a wrapper (measureclient formats
// the transport error with %v) has already rendered the URL into its own
// message, where rewriting the *url.Error cannot reach it.
func sanitizeError(err error, sentinels ...error) error {
	if err == nil {
		return nil
	}
	out := &safeError{msg: urlInText.ReplaceAllStringFunc(err.Error(), SafeURL)}
	for _, s := range sentinels {
		if errors.Is(err, s) {
			out.wrap = append(out.wrap, s)
		}
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		out.wrap = append(out.wrap, &url.Error{Op: ue.Op, URL: SafeURL(ue.URL), Err: sanitizeError(ue.Err, sentinels...)})
	}
	return out
}
