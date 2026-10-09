package workerworkattribution

import "crypto/sha256"

// A revision edge must check readability and include both file identity and a
// change timestamp that ordinary writes cannot preserve by restoring mtime.
// Empty revisions decline reuse. Injected content readers without this contract
// continue to read and digest the exact artifact on every fresh request.
func (r *ArtifactHistoryReader) sourceRevision(key historyIdentity) (string, error) {
	if r.revision == nil {
		return "", nil
	}
	revision, err := r.revision(key.artifact)
	if err != nil {
		key.generation = ""
		r.mu.Lock()
		cached := r.names[key]
		cached.revision = ""
		if _, exists := r.names[key]; exists {
			r.names[key] = cached
		}
		r.mu.Unlock()
	}
	return revision, err
}

func (r *ArtifactHistoryReader) revisionProjection(key historyIdentity, revision string) (nameProjection, bool) {
	key.generation = ""
	r.mu.Lock()
	defer r.mu.Unlock()
	cached, ok := r.names[key]
	if !ok || cached.revision != revision {
		return nameProjection{}, false
	}
	r.nameUse++
	cached.lastUse = r.nameUse
	r.names[key] = cached
	return cached.projection, true
}

// Check again after read/decode: a changed source must not label an earlier
// projection with its newer revision. Retain only if the cache still contains
// the same digest; another concurrent reader may have already replaced it.
func (r *ArtifactHistoryReader) retainSourceRevision(key historyIdentity, digest [sha256.Size]byte, revision string) {
	after, err := r.sourceRevision(key)
	if err != nil || after != revision {
		return
	}
	key.generation = ""
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.names[key]; ok && cached.digest == digest {
		cached.revision = revision
		r.names[key] = cached
	}
}
