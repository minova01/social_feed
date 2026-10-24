package domainpost

import "errors"

var ErrPostNotFound = errors.New("post not found")
var ErrPostPermissionDenied = errors.New("post permission denied")
var ErrDuplicateIdempotencyKey = errors.New("duplicate post idempotency key")
