package errdefs

import (
	"context"
	"errors"
)

// Definitions of common error types used. All errors returned by
// most packages will map into one of these errors classes.
// Packages should return errors of these types when they want to instruct a
// client to take a particular action.
//
// For the most part, we just try to provide local grpc errors. Most conditions
// map very well to those defined by grpc.
var (
	ErrUnknown            = errors.New("unknown") // used internally to represent a missed mapping.
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrNotFound           = errors.New("not found")
	ErrAlreadyExists      = errors.New("already exists")
	ErrFailedPrecondition = errors.New("failed precondition")
	ErrUnavailable        = errors.New("unavailable")
	ErrNotImplemented     = errors.New("not implemented") // represents not supported and unimplemented
)

// IsInvalidArgument returns true if the error is due to an invalid argument
func IsInvalidArgument(err error) bool {
	return errors.Is(err, ErrInvalidArgument)
}

// IsNotFound returns true if the error is due to a missing object
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists returns true if the error is due to an already existing
// metadata item
func IsAlreadyExists(err error) bool {
	return errors.Is(err, ErrAlreadyExists)
}

// IsFailedPrecondition returns true if an operation could not proceed to the
// lack of a particular condition
func IsFailedPrecondition(err error) bool {
	return errors.Is(err, ErrFailedPrecondition)
}

// IsUnavailable returns true if the error is due to a resource being unavailable
func IsUnavailable(err error) bool {
	return errors.Is(err, ErrUnavailable)
}

// IsNotImplemented returns true if the error is due to not being implemented
func IsNotImplemented(err error) bool {
	return errors.Is(err, ErrNotImplemented)
}

// IsCanceled returns true if the error is due to `context.Canceled`.
func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}

// IsDeadlineExceeded returns true if the error is due to
// `context.DeadlineExceeded`.
func IsDeadlineExceeded(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
