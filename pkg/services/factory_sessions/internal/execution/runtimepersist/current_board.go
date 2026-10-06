package runtimepersist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

const currentBoardSchemaVersion = "factory-sessions.current-board.v1"

// currentBoardReference is a repository-scoped pointer, never board content.
type currentBoardReference struct {
	SchemaVersion     string `json:"schemaVersion"`
	FactoryDirectory  string `json:"factoryDirectory"`
	FactorySessionID  string `json:"factorySessionId"`
	ArtifactReference string `json:"artifactReference"`
}

// CurrentBoardStore uses the same injected atomic filesystem as snapshots.
// Reading it does not create directories or rewrite invalid references.
type CurrentBoardStore interface {
	LoadCurrentBoard(context.Context, string) (string, error)
	SaveCurrentBoard(context.Context, string, string) error
}

func (s DirectoryStore) currentBoardPath() string {
	return filepath.Join(filepath.Dir(s.Dir), "current-board.json")
}

// LoadCurrentBoard validates the exact persisted contract before returning a
// local artifact reference. Recordings remains responsible for its contents.
func (s DirectoryStore) LoadCurrentBoard(ctx context.Context, factoryDirectory string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := s.files.ReadFile(s.currentBoardPath())
	if cancelErr := ctx.Err(); cancelErr != nil {
		return "", cancelErr
	}
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", &persistenceError{operation: "read current board reference", cause: err}
	}
	reference, err := decodeCurrentBoardReference(data)
	if err == nil {
		err = reference.validate(factoryDirectory)
	}
	if err != nil {
		return "", &persistenceError{operation: "validate current board reference", cause: err}
	}
	return reference.ArtifactReference, nil
}

// SaveCurrentBoard publishes only a validated reference. Atomic replacement is
// supplied by the canonical RuntimePersistenceFileSystem, as for snapshots.
func (s DirectoryStore) SaveCurrentBoard(ctx context.Context, factoryDirectory, artifact string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	reference := currentBoardReference{
		SchemaVersion: currentBoardSchemaVersion, FactoryDirectory: factoryDirectory,
		FactorySessionID: "~default", ArtifactReference: artifact,
	}
	if err := reference.validate(factoryDirectory); err != nil {
		return &persistenceError{operation: "validate current board reference", cause: err}
	}
	data, err := json.Marshal(reference)
	if err != nil {
		return err
	}
	if err := s.files.MkdirAll(filepath.Dir(s.currentBoardPath()), 0o700); err != nil {
		return &persistenceError{operation: "create current board reference directory", cause: err}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.files.WriteFile(s.currentBoardPath(), data, 0o600); err != nil {
		return &persistenceError{operation: "write current board reference", cause: err}
	}
	return nil
}

func (reference currentBoardReference) validate(factoryDirectory string) error {
	if reference.SchemaVersion != currentBoardSchemaVersion || reference.FactorySessionID != "~default" {
		return errors.New("unsupported current board reference version or session")
	}
	if !filepath.IsAbs(factoryDirectory) || !filepath.IsAbs(reference.FactoryDirectory) ||
		filepath.Clean(reference.FactoryDirectory) != filepath.Clean(factoryDirectory) {
		return errors.New("current board reference belongs to another Factory directory")
	}
	if strings.TrimSpace(reference.ArtifactReference) != reference.ArtifactReference ||
		!filepath.IsAbs(reference.ArtifactReference) {
		return errors.New("current board reference requires a local absolute artifact path")
	}
	return nil
}

// Decode members explicitly: encoding/json otherwise accepts duplicate names
// and case-insensitive spellings, which are outside this reference contract.
func decodeCurrentBoardReference(data []byte) (currentBoardReference, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return currentBoardReference{}, errors.New("current board reference must be an object")
	}
	var reference currentBoardReference
	fields := map[string]*string{
		"schemaVersion": &reference.SchemaVersion, "factoryDirectory": &reference.FactoryDirectory,
		"factorySessionId": &reference.FactorySessionID, "artifactReference": &reference.ArtifactReference,
	}
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return currentBoardReference{}, errors.New("invalid current board reference member")
		}
		name, ok := key.(string)
		field := fields[name]
		if !ok || field == nil || seen[name] {
			return currentBoardReference{}, errors.New("unknown or duplicate current board reference member")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return currentBoardReference{}, errors.New("invalid current board reference value")
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return currentBoardReference{}, errors.New("current board reference values must be nonempty strings")
		}
		*field, seen[name] = text, true
	}
	if _, err := decoder.Token(); err != nil || len(seen) != len(fields) {
		return currentBoardReference{}, errors.New("incomplete current board reference")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return currentBoardReference{}, errors.New("unexpected data after current board reference")
	}
	return reference, nil
}
