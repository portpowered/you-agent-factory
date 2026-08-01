package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

// WriteJSON serializes one generated HTTP response and logs encoding failures.
func WriteJSON(w http.ResponseWriter, status int, value any, logger *zap.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		if logger != nil {
			logger.Error("encode response failed", zap.Error(err))
		}
	}
}

// WriteError emits the common generated ErrorResponse envelope.
func WriteError(w http.ResponseWriter, status int, message, code string, logger *zap.Logger) {
	WriteJSON(w, status, factoryapi.ErrorResponse{
		Message: message,
		Family:  errorFamilyForStatus(status),
		Code:    factoryapi.ErrorResponseCode(code),
	}, logger)
}

func errorFamilyForStatus(status int) factoryapi.ErrorFamily {
	switch status {
	case http.StatusBadRequest:
		return factoryapi.ErrorFamilyBadRequest
	case http.StatusConflict:
		return factoryapi.ErrorFamilyConflict
	case http.StatusNotFound:
		return factoryapi.ErrorFamilyNotFound
	default:
		return factoryapi.ErrorFamilyInternalServerError
	}
}

// RequestFieldValidationError is a transport decoding failure whose message is
// safe to expose to an HTTP caller.
type RequestFieldValidationError struct {
	message string
}

func (e RequestFieldValidationError) Error() string {
	return e.message
}

// RequestFieldValidationMessage extracts a safe structural validation message.
func RequestFieldValidationMessage(err error) (string, bool) {
	var validationErr RequestFieldValidationError
	if errors.As(err, &validationErr) {
		return validationErr.message, true
	}
	return "", false
}

func ensureSingleJSONObject(dec *json.Decoder) error {
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return RequestFieldValidationError{message: "request payload must contain one JSON object"}
		}
		return err
	}
	return nil
}

func ensureJSONObjectPayload(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] != '{' {
		return RequestFieldValidationError{message: "request payload must contain one JSON object"}
	}
	return nil
}

// DecodeError marks a request-body decoding failure without exposing which
// owner operation performed the decode.
type DecodeError struct {
	Cause error
}

func (e DecodeError) Error() string {
	return e.Cause.Error()
}

func (e DecodeError) Unwrap() error {
	return e.Cause
}

// IsDecodeError reports whether err originated at the HTTP request decoder.
func IsDecodeError(err error) bool {
	var decodeErr DecodeError
	return errors.As(err, &decodeErr)
}

// DecodeStrictJSON decodes exactly one JSON object and rejects unknown fields.
func DecodeStrictJSON[T any](body io.Reader) (T, error) {
	var zero T
	if body == nil {
		return zero, DecodeError{Cause: errors.New("request body is required")}
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return zero, DecodeError{Cause: err}
	}
	if err := ensureJSONObjectPayload(data); err != nil {
		return zero, DecodeError{Cause: err}
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var req T
	if err := decoder.Decode(&req); err != nil {
		return zero, DecodeError{Cause: err}
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return zero, DecodeError{Cause: err}
	}
	return req, nil
}

// DecodeOptionalJSONRequest accepts an empty body and otherwise applies the
// same strict single-object rules as DecodeStrictJSON.
func DecodeOptionalJSONRequest[T any](body io.Reader, zero func() T) (T, error) {
	if body == nil {
		return zero(), nil
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return zero(), DecodeError{Cause: err}
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return zero(), nil
	}
	if err := ensureJSONObjectPayload(trimmed); err != nil {
		return zero(), DecodeError{Cause: err}
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var req T
	if err := decoder.Decode(&req); err != nil {
		return zero(), DecodeError{Cause: err}
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return zero(), DecodeError{Cause: err}
	}
	return req, nil
}
