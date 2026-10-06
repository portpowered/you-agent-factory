package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const controlInputLimit = recordings.WorkerControlInputMaxBytes

type controlInputArtifact struct {
	Key        recordings.WorkerControlOperationKey `json:"key"`
	Generation string                               `json:"generation"`
	Input      []byte                               `json:"input"`
}

// PersistWorkerControlInput uses the same injected sync acknowledgement as the
// journal, rather than the captured preview spill's replacement API.
func (writer *FileWriter) PersistWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	if len(input) > controlInputLimit || !json.Valid(input) {
		return "", recordings.ErrInvalidWorkerControlOperation
	}
	entry := writer.entry(key.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return writer.persistWorkerControlInputLocked(ctx, entry, key, input)
}

func (writer *FileWriter) persistWorkerControlInputLocked(ctx context.Context, entry *recordingEntry, key recordings.WorkerControlOperationKey, input json.RawMessage) (string, error) {
	artifact, err := writer.controlInputIdentity(ctx, entry, key)
	if err != nil {
		return "", err
	}
	ref := controlInputRef(artifact)
	path := writer.controlInputPath(ref)
	pathErr := writer.checkControlInputPath(path)
	if pathErr != nil && !errors.Is(pathErr, os.ErrNotExist) {
		return "", pathErr
	}
	// This writer owns immutable creation under entry.mu. The directory check
	// already establishes absence; reading a new blob would invoke replacement
	// retries on Windows while every sibling control waits on the recording.
	var data []byte
	err = pathErr
	if pathErr == nil {
		data, err = writer.storage.ReadFile(path)
	}
	if err == nil {
		previous, err := decodeControlInput(data, artifact)
		if err != nil {
			return "", err
		}
		if !bytes.Equal(previous, input) {
			return "", recordings.ErrWorkerControlConflict
		}
		// A prior writer might have reported a sync failure after writing all
		// bytes. Reuse must acknowledge a sync, without duplicating JSON.
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if err := writer.appender.AppendFile(path, nil); err != nil {
			return "", err
		}
		return ref, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	artifact.Input = input
	data, err = json.Marshal(artifact)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := writer.appender.AppendFile(path, data); err != nil {
		return "", err
	}
	return ref, nil
}

func (writer *FileWriter) ReadWorkerControlInput(ctx context.Context, key recordings.WorkerControlOperationKey, ref string) (json.RawMessage, error) {
	entry := writer.entry(key.RecordingID)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	artifact, err := writer.controlInputIdentity(ctx, entry, key)
	if err != nil {
		return nil, err
	}
	if ref != controlInputRef(artifact) {
		return nil, recordings.ErrInvalidWorkerControlOperation
	}
	path := writer.controlInputPath(ref)
	if err := writer.checkControlInputPath(path); err != nil {
		return nil, err
	}
	data, err := writer.storage.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeControlInput(data, artifact)
}

// ReadWorkerContinuationInput resolves the retained generation inside the
// selected store. Callers supply identity data, never a path or cached hash.
func (writer *FileWriter) ReadWorkerContinuationInput(ctx context.Context, key recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	entry := writer.entry(key.RecordingID)
	entry.mu.Lock()
	identity, err := writer.controlInputIdentity(ctx, entry, key)
	entry.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return writer.ReadWorkerControlInput(ctx, key, controlInputRef(identity))
}

func (writer *FileWriter) controlInputIdentity(ctx context.Context, entry *recordingEntry, key recordings.WorkerControlOperationKey) (controlInputArtifact, error) {
	if key.RecordingID == "" || key.WorkerSessionID == "" || key.RequestID == "" {
		return controlInputArtifact{}, recordings.ErrInvalidWorkerControlOperation
	}
	if err := writer.hydrate(ctx, key.RecordingID, entry); err != nil {
		return controlInputArtifact{}, err
	}
	session := entry.sessions[key.WorkerSessionID]
	if entry.damaged || session == nil || len(session.records) == 0 {
		return controlInputArtifact{}, recordings.ErrWorkerRecordingReplay
	}
	if writer.catalogEntry(session).FactorySessionID != key.FactorySessionID {
		return controlInputArtifact{}, recordings.ErrWorkerControlConflict
	}
	return controlInputArtifact{Key: key, Generation: session.generation}, nil
}

func (writer *FileWriter) validateControlInput(ctx context.Context, entry *recordingEntry, record recordings.WorkerControlOperationRecord) error {
	identity, err := writer.controlInputIdentity(ctx, entry, operationKey(record))
	if err != nil {
		return err
	}
	if record.InputArtifactRef != controlInputRef(identity) {
		return recordings.ErrInvalidWorkerControlOperation
	}
	path := writer.controlInputPath(record.InputArtifactRef)
	if err := writer.checkControlInputPath(path); err != nil {
		return err
	}
	data, err := writer.storage.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = decodeControlInput(data, identity)
	return err
}

func controlInputRef(artifact controlInputArtifact) string {
	artifact.Input = nil
	identity, _ := json.Marshal(artifact)
	digest := sha256.Sum256(identity)
	return hex.EncodeToString(digest[:])
}

func (writer *FileWriter) controlInputPath(ref string) string {
	return filepath.Join(writer.root, "worker-payloads", ref+".control.json")
}

func decodeControlInput(data []byte, identity controlInputArtifact) (json.RawMessage, error) {
	var artifact controlInputArtifact
	if json.Unmarshal(data, &artifact) != nil || artifact.Key != identity.Key || artifact.Generation != identity.Generation || len(artifact.Input) > controlInputLimit || !json.Valid(artifact.Input) {
		return nil, recordings.ErrWorkerRecordingReplay
	}
	return append(json.RawMessage(nil), artifact.Input...), nil
}

// References never supply paths. Reject pre-existing links at the profile root,
// payload directory and blob through the injected filesystem scanner as well.
// The configured profile's ancestors remain the composition boundary's trust.
func (writer *FileWriter) checkControlInputPath(path string) error {
	exists := false
	for _, candidate := range []string{filepath.Clean(writer.root), filepath.Dir(path), path} {
		err := writer.directory.ScanDirectory(filepath.Dir(candidate), 64, func(files []os.DirEntry) error {
			for _, file := range files {
				if file.Name() == filepath.Base(candidate) {
					if file.Type()&os.ModeSymlink != 0 {
						return recordings.ErrInvalidWorkerControlOperation
					}
					if candidate == path {
						exists = true
					}
				}
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !exists {
		return os.ErrNotExist
	}
	return nil
}
