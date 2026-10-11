package agentmessages

import "github.com/portpowered/infinite-you/pkg/platform/filesystem"

// StoreFileSystem is the policy-free filesystem capability for the independent
// message journal. Messaging owns transaction format, rollback and retention;
// the host supplies durable, seekable and truncatable descriptors from OpenFile.
// ReplaceDurable flushes private bytes before atomic publication and preserves
// the previous file on failure. Separate processes must not share a writable
// profile. No Factory execution or recording effect belongs to this boundary.
type StoreFileSystem = filesystem.DurableJournalFileSystem
