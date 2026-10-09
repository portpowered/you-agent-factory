//go:build !windows && !linux

package filesystem

// ReadRevision declines reuse on hosts without a supported change-time and
// file-identity check. Consumers must read content when the revision is empty.
func (RevisionReader) ReadRevision(string) (string, error) { return "", nil }
