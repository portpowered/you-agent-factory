package factorysessions

import "io/fs"

// RuntimePersistenceStore is the durable snapshot role consumed by Factory
// Session execution. Its implementation remains private to Factory Sessions.
type RuntimePersistenceStore interface {
	Save(sessionID string, encoded []byte) error
	Load(sessionID string) ([]byte, error)
}

// RuntimePersistenceFileSystem is the exact host-filesystem capability used
// after Factory Sessions has selected snapshot paths and permissions.
type RuntimePersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
}

// RuntimePersistenceStoreFactory opens the project-local durable snapshot
// store from external mechanics selected once by Wire.
type RuntimePersistenceStoreFactory func(projectRoot string) (RuntimePersistenceStore, error)
