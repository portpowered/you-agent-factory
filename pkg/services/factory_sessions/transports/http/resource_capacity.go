package http

import (
	"errors"
	"net/http"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// SetFactorySessionResourceCapacity handles one live resource-capacity change.
// Resource identity comes from the stable resource_id path parameter; the
// mutable display name is never accepted as an alternate target.
func (s *Server) SetFactorySessionResourceCapacity(
	w http.ResponseWriter,
	r *http.Request,
	sessionID factoryapi.SessionID,
	resourceID factoryapi.ResourceID,
) {
	if !requestAcceptsJSONContentType(r.Header.Get("Content-Type")) {
		s.writeUnsupportedMediaTypeError(w)
		return
	}
	if s.sessionsRoot == nil {
		s.writeError(w, http.StatusInternalServerError, "live change service is unavailable", "INTERNAL_ERROR")
		return
	}
	requestBody, err := decodeStrictJSON[factoryapi.FactorySessionResourceCapacityRequest](r.Body)
	if err != nil {
		if message, ok := requestFieldValidationMessage(err); ok {
			s.writeError(w, http.StatusBadRequest, message, "BAD_REQUEST")
			return
		}
		s.writeError(w, http.StatusBadRequest, "invalid request payload", "BAD_REQUEST")
		return
	}
	source := factorysession.ResourceCapacitySourceAPI
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-You-Source")), factorysession.ResourceCapacitySourceCLI) {
		source = factorysession.ResourceCapacitySourceCLI
	}
	request, err := factorysession.ResourceCapacityRequestFromAPIWithSource(string(resourceID), requestBody, source)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	if s.guardSessionsRequestContext(w, r) {
		return
	}
	result, err := s.sessionsRoot.ApplyLiveChange(r.Context(), string(sessionID), request)
	if err != nil {
		s.writeLiveChangeError(w, string(sessionID), err)
		return
	}
	response := factorysession.ResourceCapacityResponseToAPI(result)
	response.Links = factorysession.ResourceCapacityResponseLinks(string(sessionID))
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Server) writeLiveChangeError(w http.ResponseWriter, sessionID string, err error) {
	var liveChangeErr *factorysessions.LiveChangeError
	if !errors.As(err, &liveChangeErr) {
		if errors.Is(err, factorysessions.ErrSessionNotFound) || errors.Is(err, factorysessions.ErrLiveChangeSessionNotFound) {
			s.writeError(w, http.StatusNotFound, "factory session not found", string(factoryapi.ErrorResponseCodeNOTFOUND))
			return
		}
		s.writeSessionsRootErrorOrInternal(w, sessionID, err, "live change application failed")
		return
	}

	status := http.StatusInternalServerError
	code := string(factoryapi.ErrorResponseCodeINTERNALERROR)
	switch liveChangeErr.Code {
	case factorysessions.LiveChangeErrorInvalidRequest:
		status, code = http.StatusBadRequest, string(factoryapi.ErrorResponseCodeBADREQUEST)
	case factorysessions.LiveChangeErrorSessionNotFound,
		factorysessions.LiveChangeErrorTargetNotFound:
		status, code = http.StatusNotFound, string(factoryapi.ErrorResponseCodeNOTFOUND)
	case factorysessions.LiveChangeErrorRevisionConflict:
		status, code = http.StatusConflict, string(factoryapi.ErrorResponseCodeREVISIONCONFLICT)
	case factorysessions.LiveChangeErrorLifecycleConflict:
		status, code = http.StatusConflict, string(factoryapi.ErrorResponseCodeLIFECYCLECONFLICT)
	case factorysessions.LiveChangeErrorCapacityInUse:
		status, code = http.StatusConflict, string(factoryapi.ErrorResponseCodeRESOURCECAPACITYINUSE)
	case factorysessions.LiveChangeErrorRequestConflict:
		status, code = http.StatusConflict, string(factoryapi.ErrorResponseCodeREQUESTCONFLICT)
	case factorysessions.LiveChangeErrorApplicationFailed:
		status, code = http.StatusInternalServerError, string(factoryapi.ErrorResponseCodeADMITTEDAPPLICATIONFAILURE)
	}
	message := strings.TrimSpace(liveChangeErr.Error())
	if message == "" {
		message = "live change application failed"
	}
	if liveChangeErr.Code == factorysessions.LiveChangeErrorCapacityInUse && liveChangeErr.ResourceCapacity != nil {
		s.writeJSON(w, status, factoryapi.ErrorResponse{
			Message:          message,
			Family:           errorFamilyForStatus(status),
			Code:             factoryapi.ErrorResponseCode(code),
			ResourceCapacity: factorysession.ResourceCapacityErrorDetailsToAPI(liveChangeErr.ResourceCapacity),
		})
		return
	}
	s.writeError(w, status, message, code)
}
