package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type recordingCatalogEntry struct {
	key        string
	projection recordings.WorkerRecordingProjection
}

func (writer *FileWriter) scanWorkerRecordingCatalog(
	ctx context.Context,
	request recordings.WorkerRecordingListRequest,
	cursor string,
) ([]recordingCatalogEntry, []recordings.WorkerRecordingCatalogDiagnostic, error) {
	if writer.readDir == nil {
		return nil, nil, recordings.ErrMissingWorkerRecordingReader
	}
	entries, err := writer.readDir(writer.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("list Worker recording home: %w", err)
	}

	all := make([]recordingCatalogEntry, 0)
	diagnostics := make([]recordings.WorkerRecordingCatalogDiagnostic, 0)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if entry.IsDir() || !isWorkerRecordingArtifactName(entry.Name()) {
			continue
		}
		path := filepath.Join(writer.root, entry.Name())
		snapshot, readErr := writer.readArtifactPath(ctx, path, "")
		if readErr != nil {
			appendCatalogDiagnostic(&diagnostics, recordings.WorkerRecordingCatalogDiagnostic{
				Path: path, Code: catalogDiagnosticCode(readErr), Message: catalogDiagnosticMessage(readErr),
			})
			continue
		}
		for _, session := range snapshot.Sessions {
			result, replayErr := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
				Snapshot: snapshot, WorkerSessionID: session.WorkerSessionID,
			})
			if replayErr != nil {
				appendCatalogDiagnostic(&diagnostics, recordings.WorkerRecordingCatalogDiagnostic{
					RecordingID: snapshot.RecordingID, Path: path,
					Code: catalogDiagnosticCode(replayErr), Message: catalogDiagnosticMessage(replayErr),
				})
				continue
			}
			value := result.Projection
			if diagnostic, ok := catalogProjectionDiagnostic(value, path); ok {
				appendCatalogDiagnostic(&diagnostics, diagnostic)
			}
			if !catalogRequestMatches(value, request) {
				continue
			}
			key := recordingCatalogKey(value)
			if key > cursor {
				all = append(all, recordingCatalogEntry{key: key, projection: value})
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].key < all[j].key })
	return all, diagnostics, nil
}

func isWorkerRecordingArtifactName(name string) bool {
	return strings.HasSuffix(name, workerRecordingV2Suffix) || strings.HasSuffix(name, ".worker.json")
}

func catalogRequestMatches(projection recordings.WorkerRecordingProjection, request recordings.WorkerRecordingListRequest) bool {
	if request.FactorySessionID != "" && projection.FactorySessionID != request.FactorySessionID {
		return false
	}
	return request.WorkID == "" || containsString(projection.WorkIDs, request.WorkID)
}

func appendCatalogDiagnostic(
	diagnostics *[]recordings.WorkerRecordingCatalogDiagnostic,
	diagnostic recordings.WorkerRecordingCatalogDiagnostic,
) {
	if diagnostics == nil || len(*diagnostics) >= maxWorkerRecordingDiagnostics {
		return
	}
	*diagnostics = append(*diagnostics, diagnostic)
}

func recordingCatalogKey(projection recordings.WorkerRecordingProjection) string {
	return projection.WorkerSessionID + "\x00" + projection.RecordingID
}

func catalogProjectionPage(entries []recordingCatalogEntry) []recordings.WorkerRecordingProjection {
	values := make([]recordings.WorkerRecordingProjection, len(entries))
	for index, entry := range entries {
		values[index] = cloneProjection(entry.projection)
	}
	return values
}

func catalogDiagnosticCode(err error) recordings.WorkerRecordingCatalogDiagnosticCode {
	switch {
	case errors.Is(err, recordings.ErrWorkerRecordingCorruptTail):
		return recordings.WorkerRecordingCatalogMalformedTail
	case errors.Is(err, recordings.ErrWorkerRecordingCompatibility):
		return recordings.WorkerRecordingCatalogUnsupported
	case errors.Is(err, recordings.ErrWorkerRecordingRetention):
		return recordings.WorkerRecordingCatalogRetention
	default:
		return recordings.WorkerRecordingCatalogUnreadable
	}
}

func catalogDiagnosticMessage(err error) string {
	if errors.Is(err, recordings.ErrWorkerRecordingCorruptTail) {
		return "valid Worker recording prefix retained; tail is not readable"
	}
	if errors.Is(err, recordings.ErrWorkerRecordingCompatibility) {
		return "Worker recording schema is not supported"
	}
	return "Worker recording artifact could not be read"
}

func catalogProjectionDiagnostic(
	projection recordings.WorkerRecordingProjection,
	path string,
) (recordings.WorkerRecordingCatalogDiagnostic, bool) {
	reason := strings.TrimSpace(projection.Degradation)
	if reason == "" {
		reason = strings.TrimSpace(projection.InterruptionReason)
	}
	code := recordings.WorkerRecordingCatalogDiagnosticCode(reason)
	switch code {
	case recordings.WorkerRecordingCatalogMalformedTail:
		return recordings.WorkerRecordingCatalogDiagnostic{
			RecordingID: projection.RecordingID, Path: path,
			Code:    recordings.WorkerRecordingCatalogMalformedTail,
			Message: "valid Worker recording prefix retained; tail is not readable",
		}, true
	case recordings.WorkerRecordingCatalogUnsupported:
		return recordings.WorkerRecordingCatalogDiagnostic{
			RecordingID: projection.RecordingID, Path: path,
			Code:    recordings.WorkerRecordingCatalogUnsupported,
			Message: "Worker recording schema is not supported",
		}, true
	default:
		return recordings.WorkerRecordingCatalogDiagnostic{}, false
	}
}
