package certmanager

import "errors"

// ErrTokenRequired is returned when create is attempted without an API token.
var ErrTokenRequired = errors.New("api token is required")
