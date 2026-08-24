package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func parseGenericSource(raw string) (genericSource, error) {
	lower := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(lower, "hf://"):
		return parseGenericHFSource(raw)
	case strings.HasPrefix(lower, "file://"):
		return parseGenericFileSource(raw)
	case strings.HasPrefix(lower, "https://"):
		return parseGenericReleaseSource(raw)
	case strings.Contains(raw, "://"):
		return genericSource{}, models.ErrAssetSourceUnsupported
	case looksLikeLocalPath(raw):
		return genericSource{kind: genericSourceLocal, safe: "local://path", localPath: raw}, nil
	default:
		return genericSource{}, models.ErrAssetSourceUnsupported
	}
}

func parseGenericReleaseSource(raw string) (genericSource, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		!strings.Contains(path.Clean(parsed.Path), "/releases/download/") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\\\x00") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	checksum := sha256.Sum256([]byte(raw))
	return genericSource{
		kind: genericSourceRelease, safe: "release://" + hex.EncodeToString(checksum[:]),
		artifactURL: raw, revision: hex.EncodeToString(checksum[:]),
	}, nil
}

func parseGenericHFSource(raw string) (genericSource, error) {
	rest := strings.TrimSpace(raw[len("hf://"):])
	if rest == "" || strings.ContainsAny(rest, "\x00?#\\") {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	at := strings.LastIndex(rest, "@")
	base, revision := rest, ""
	if at >= 0 {
		base, revision = rest[:at], rest[at+1:]
	}
	parts := strings.Split(base, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." || strings.ContainsAny(part, " @\t\r\n") {
			return genericSource{}, models.ErrModelReferenceInvalid
		}
	}
	if strings.ContainsAny(revision, "\x00/@\\?# \t\r\n") {
		return genericSource{}, models.ErrModelRevisionUnresolved
	}
	file := strings.Join(parts[2:], "/")
	safe := "hf://" + parts[0] + "/" + parts[1]
	if file != "" {
		safe += "/" + file
	}
	if revision != "" {
		safe += "@" + revision
	}
	return genericSource{
		kind: genericSourceHF, safe: safe, owner: parts[0], repository: parts[1],
		file: file, revision: revision,
	}, nil
}

func genericHFSafeReference(source genericSource) string {
	safe := "hf://" + source.owner + "/" + source.repository
	if source.file != "" {
		safe += "/" + source.file
	}
	return safe + "@" + source.revision
}

func genericRevisionFailure() error {
	return &models.InvocationFailure{
		Class:   models.InvocationFailureClassRevisionResolution,
		Message: "model source revision could not be resolved to an immutable commit",
		Cause:   models.ErrModelRevisionUnresolved,
	}
}

func parseGenericFileSource(raw string) (genericSource, error) {
	parsed, err := url.Parse(raw)
	if err != nil || !validGenericFileURL(parsed) {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	localPath, err := genericFileURLPath(parsed)
	if err != nil {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	if localPath == "" {
		return genericSource{}, models.ErrModelReferenceInvalid
	}
	return genericSource{
		kind: genericSourceFile, safe: "file://local", localPath: filepath.FromSlash(localPath),
	}, nil
}

func validGenericFileURL(parsed *url.URL) bool {
	return parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func genericFileURLPath(parsed *url.URL) (string, error) {
	localPath, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		if len(parsed.Host) != 2 || parsed.Host[1] != ':' || !isASCIIAlphaByte(parsed.Host[0]) {
			return "", models.ErrModelReferenceInvalid
		}
		localPath = parsed.Host + localPath
	}
	if len(localPath) >= 3 && localPath[0] == '/' && localPath[2] == ':' && isASCIIAlphaByte(localPath[1]) {
		localPath = localPath[1:]
	}
	return localPath, nil
}

func isImmutableGenericRevision(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *service) genericAssetURL(source genericSource, name string) string {
	if source.kind == genericSourceRelease {
		return source.artifactURL
	}
	return strings.TrimRight(s.endpoints.BaseURL, "/") + "/" + source.owner + "/" +
		source.repository + "/resolve/" + source.revision + "/" + url.PathEscape(name) + "?download=true"
}

func (s *service) addGenericURLs(source genericSource, artifacts []genericArtifact) {
	if source.kind != genericSourceHF {
		return
	}
	for index := range artifacts {
		if artifacts[index].url == "" {
			artifacts[index].url = s.genericAssetURL(source, artifacts[index].requirement.Name)
		}
	}
}

func sourceDisplayName(source genericSource) string {
	if source.kind == genericSourceHF {
		return source.owner + "/" + source.repository
	}
	return "local-model"
}

func sourceMetadata(source genericSource) models.SourceMetadata {
	if source.kind == genericSourceHF {
		return models.SourceMetadata{Provider: "HUGGINGFACE", Reference: source.owner + "/" + source.repository, Revision: source.revision}
	}
	if source.kind == genericSourceRelease {
		return models.SourceMetadata{Provider: "PINNED_BACKEND", Reference: "pinned-backend", Revision: source.revision}
	}
	return models.SourceMetadata{Provider: "LOCAL", Reference: source.safe}
}

func genericCacheKey(kind string, source genericSource, artifacts []genericArtifact) string {
	names := genericArtifactIdentityNames(artifacts)
	sort.Strings(names)
	return kind + "|" + genericSourceIdentity(source) + "|" + strings.Join(names, ",")
}

func genericSourceIdentity(source genericSource) string {
	if source.kind != genericSourceLocal && source.kind != genericSourceFile {
		return source.safe
	}
	checksum := sha256.Sum256([]byte(source.localPath))
	return source.safe + "|" + hex.EncodeToString(checksum[:])
}

func genericArtifactIdentityNames(artifacts []genericArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		requirement := artifact.requirement
		names = append(names, fmt.Sprintf(
			"%s:%d:%s",
			requirement.Name,
			requirement.Bytes,
			strings.ToLower(strings.TrimSpace(requirement.SHA256)),
		))
	}
	return names
}

func genericArtifactIdentityHash(kind string, source genericSource, artifacts []genericArtifact) string {
	return genericIdentityHash(kind, source, genericArtifactIdentityNames(artifacts))
}

func genericIdentityHash(kind string, source genericSource, names []string) string {
	identity := kind + "|" + genericSourceIdentity(source)
	if len(names) > 0 {
		cloned := append([]string(nil), names...)
		sort.Strings(cloned)
		identity += "|" + strings.Join(cloned, ",")
	}
	hash := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(hash[:])
}

func missingArtifactNames(artifacts []genericArtifact) []string {
	names := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		names = append(names, artifact.requirement.Name)
	}
	sort.Strings(names)
	return uniqueStrings(names)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func isASCIIAlphaByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func looksLikeLocalPath(value string) bool {
	return filepath.IsAbs(value) || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") ||
		strings.HasPrefix(value, ".\\") || strings.HasPrefix(value, "..\\") ||
		strings.ContainsAny(value, `/\\`) || filepath.Ext(value) != ""
}
