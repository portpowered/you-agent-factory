package factory

import "errors"

var (
	// ErrMoveWorkNotFound indicates no work token matches the requested work ID.
	ErrMoveWorkNotFound = errors.New("work not found")
	// ErrMoveWorkInvalidState indicates the target state is unknown for the work type.
	ErrMoveWorkInvalidState = errors.New("invalid target state for work type")
	// ErrMoveWorkInFlightDispatch indicates the work item is consumed by an active dispatch.
	ErrMoveWorkInFlightDispatch = errors.New("work is in an active dispatch")
	// ErrMoveWorkEngineTerminated indicates the engine no longer accepts control ingress.
	ErrMoveWorkEngineTerminated = errors.New("engine has terminated")
)
