package incomingqueuestore

import "errors"

var errMissingClient = errors.New("incoming queue redis client is not configured")
