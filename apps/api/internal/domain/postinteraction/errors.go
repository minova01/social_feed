package domainpostinteraction

import "errors"

var ErrPostNotFound = errors.New("post not found")
var ErrCommentNotFound = errors.New("comment not found")
var ErrCommentPermissionDenied = errors.New("comment permission denied")
var ErrDuplicateCommentIdempotencyKey = errors.New("duplicate comment idempotency key")
