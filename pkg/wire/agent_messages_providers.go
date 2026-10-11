package wire

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/google/uuid"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	agentmessageswire "github.com/portpowered/infinite-you/pkg/services/agent_messages/wire"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionswire "github.com/portpowered/infinite-you/pkg/services/worker_sessions/wire"
)

func provideMessageJournal(edges serviceedges.Edges) (*agentmessageswire.Journal, error) {
	files := edges.AgentMessageStoreFileSystem
	if files == nil {
		files = platformfilesystem.Local{}
	}
	return agentmessageswire.NewJournal(files, "")
}

func provideMessageStartup(journal *agentmessageswire.Journal, clock platformclock.Source, logger logging.Logger) (*agentmessageswire.Startup, error) {
	return agentmessageswire.NewStartup(true, journal, clock.Now, 30*24*time.Hour, logger)
}

func provideMessageAuthority(validate workersessions.CallerValidator, fleet *workersessionswire.FleetObservationService) (agentmessageswire.Authority, error) {
	return agentmessageswire.NewAuthority(validate, fleet.GetObservationByWorkerSessionID, fleet.GetCapturedObservation)
}

func provideMessageValidator() (agentmessageswire.InputValidator, error) {
	return agentmessageswire.NewValidator(8192, 86400, recordings.RedactDeclaredSecretText)
}

func provideMessageQuota() (agentmessageswire.Quota, error) {
	return agentmessageswire.NewQuota(10, 8, 3)
}

func provideMessageEngine(authority agentmessageswire.Authority, validator agentmessageswire.InputValidator, quota agentmessageswire.Quota, journal *agentmessageswire.Journal, clock platformclock.Source, scheduler platformclock.TimerSource, stream events.Service, logger logging.Logger, sessions workersessions.Service) (*agentmessageswire.Engine, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return agentmessageswire.NewService(true, authority, validator, quota, journal, clock.Now, scheduler, func() string {
		id := uuid.New()
		millis := uint64(clock.Now().UnixMilli())
		for i := 5; i >= 0; i-- {
			id[i] = byte(millis)
			millis >>= 8
		}
		return "msg-" + messageIDText(id[:])
	}, key, stream, logger, sessions.Continue, sessions.Get, sessions.StreamObservationsByWorkerSessionID)
}

func messageIDText(bytes []byte) string {
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	value := new(big.Int).SetBytes(bytes)
	var encoded [26]byte
	for i := len(encoded) - 1; i >= 0; i-- {
		encoded[i] = alphabet[value.Uint64()&31]
		value.Rsh(value, 5)
	}
	return string(encoded[:])
}
