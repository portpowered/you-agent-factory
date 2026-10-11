package run

import (
	"errors"
	"net/http"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func factoryCallerInvalid() error {
	return &InvocationError{Code: "WORKER_SESSION_CALLER_INVALID", Message: "Worker Session caller credentials are invalid", Cause: workersessions.ErrCallerInvalid}
}

// Only admission requests carry credentials. Result and event reads remain
// ordinary reads, and generated request bodies never contain this authority.
func bindFactoryCaller(request *http.Request, caller *workersessions.CallerIdentity) error {
	if caller == nil {
		return nil
	}
	headers := make(http.Header)
	headers.Set("X-You-Worker-Session-Id", caller.WorkerSessionID)
	headers.Set("Authorization", "Bearer "+caller.Token)
	if _, err := apisurface.WorkerSessionCallerFromHeaders(headers); err != nil {
		return factoryCallerInvalid()
	}
	request.Header.Set("X-You-Worker-Session-Id", caller.WorkerSessionID)
	request.Header.Set("Authorization", "Bearer "+caller.Token)
	return nil
}

// An external transport can quote request headers in an error. Preserve safe
// typed failures while removing any credential-bearing diagnostic and cause.
func sanitizeFactoryCallerError(caller *workersessions.CallerIdentity, err error) error {
	if caller == nil || caller.Token == "" || err == nil {
		return err
	}
	redact := func(value string) string { return strings.ReplaceAll(value, caller.Token, "[REDACTED]") }
	var typed *InvocationError
	if errors.As(err, &typed) {
		safe := *typed
		safe.Code, safe.Message = redact(typed.Code), redact(typed.Message)
		if typed.Cause != nil && strings.Contains(typed.Cause.Error(), caller.Token) {
			safe.Cause = errors.New(redact(typed.Cause.Error()))
		}
		return &safe
	}
	if strings.Contains(err.Error(), caller.Token) {
		return errors.New(redact(err.Error()))
	}
	return err
}
