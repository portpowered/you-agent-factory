package contractvalidator

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type disabledURLLoader struct{}

func (disabledURLLoader) Load(resourceURL string) (any, error) {
	return nil, fmt.Errorf("external schema loading is disabled: %s", resourceURL)
}

type referenceResolver struct {
	root                               string
	documents                          map[string]any
	active                             map[string]bool
	allowed                            map[string]struct{}
	deduplicateResources               bool
	rejectUnsupportedReferenceKeywords bool
	resolvedResourceIDs                map[string]resourceLocation
}

type resourceLocation struct {
	document string
	segments []string
}

// LoadAndResolve loads one authored document through the validator's reviewed
// repository boundary and resolves references only to the explicitly supplied
// authored documents. It is intended for repository build tooling such as the
// contract joiner, not runtime consumers.
func LoadAndResolve(repositoryRoot, document string, authoredDocuments []string) (any, []Diagnostic) {
	value, issue := loadJSON(repositoryRoot, document, "document")
	if issue != nil {
		return nil, []Diagnostic{*issue}
	}
	allowed := make(map[string]struct{}, len(authoredDocuments)+1)
	allowed[normalizeRepositoryPath(document)] = struct{}{}
	for _, path := range authoredDocuments {
		allowed[normalizeRepositoryPath(path)] = struct{}{}
	}
	resolved, _, diagnostics := resolveReferencesWithin(repositoryRoot, document, value, allowed, true)
	sortDiagnostics(diagnostics)
	return resolved, diagnostics
}

// ValidateAuthoredPaths rejects authored inputs that escape the repository or
// resolve into a generated directory. Missing paths remain the responsibility
// of the loader so roots and referenced components retain their specific read
// diagnostics.
func ValidateAuthoredPaths(repositoryRoot string, documents []string, generatedDirectory string) []Diagnostic {
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return []Diagnostic{newDiagnostic("reference.root", rootPath, "repository root could not be resolved", "repository")}
	}
	generated := filepath.Join(root, filepath.FromSlash(normalizeRepositoryPath(generatedDirectory)))
	canonicalGenerated := generated
	if resolved, resolveErr := filepath.EvalSymlinks(generated); resolveErr == nil {
		canonicalGenerated = resolved
	}
	var diagnostics []Diagnostic
	for _, document := range documents {
		document = normalizeRepositoryPath(document)
		candidate := filepath.Join(root, filepath.FromSlash(document))
		if !containedBy(root, candidate) {
			diagnostics = append(diagnostics, newDiagnostic("join.input.escape", rootPath, "authored input escapes the repository root", document))
			continue
		}
		if containedBy(generated, candidate) {
			diagnostics = append(diagnostics, newDiagnostic("join.input.generated", rootPath, "authored input is inside generated joined output", document))
			continue
		}
		canonical, resolveErr := filepath.EvalSymlinks(candidate)
		if resolveErr != nil {
			continue
		}
		if !containedBy(root, canonical) {
			diagnostics = append(diagnostics, newDiagnostic("join.input.escape", rootPath, "authored input resolves outside the repository root", document))
			continue
		}
		if containedBy(canonicalGenerated, canonical) {
			diagnostics = append(diagnostics, newDiagnostic("join.input.generated", rootPath, "authored input resolves inside generated joined output", document))
		}
	}
	sortDiagnostics(diagnostics)
	return diagnostics
}

func resolveReferences(repositoryRoot, document string, value any) (any, []loadedDocument, []Diagnostic) {
	return resolveReferencesWithin(repositoryRoot, document, value, nil, false)
}

func resolveReferencesWithin(
	repositoryRoot, document string,
	value any,
	allowed map[string]struct{},
	deduplicateResources bool,
) (any, []loadedDocument, []Diagnostic) {
	root, err := canonicalRoot(repositoryRoot)
	if err != nil {
		return nil, nil, []Diagnostic{newDiagnostic("reference.root", rootPath, "repository root could not be resolved", document)}
	}
	document = normalizeRepositoryPath(document)
	resolver := referenceResolver{
		root:                               root,
		documents:                          map[string]any{document: value},
		active:                             make(map[string]bool),
		allowed:                            allowed,
		deduplicateResources:               deduplicateResources,
		rejectUnsupportedReferenceKeywords: allowed != nil,
		resolvedResourceIDs:                make(map[string]resourceLocation),
	}
	resolved, diagnostics := resolver.resolveNode(value, document, nil, nil, documentBaseURI(document))
	if len(diagnostics) != 0 {
		return nil, nil, diagnostics
	}
	paths := make([]string, 0, len(resolver.documents))
	for path := range resolver.documents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	loaded := make([]loadedDocument, 0, len(paths))
	for _, path := range paths {
		loaded = append(loaded, loadedDocument{path: path, value: resolver.documents[path]})
	}
	return resolved, loaded, nil
}

func (r *referenceResolver) resolveNode(value any, document string, segments, sourceSegments []string, baseURI *url.URL) (any, []Diagnostic) {
	switch typed := value.(type) {
	case map[string]any:
		if r.rejectUnsupportedReferenceKeywords {
			if diagnostics := r.unsupportedReferenceKeywordDiagnostics(typed, document, segments); len(diagnostics) != 0 {
				return nil, diagnostics
			}
		}
		resourceID, resourceURI, hasResourceID := stableResourceID(typed, baseURI)
		if r.deduplicateResources && hasResourceID {
			location := resourceLocation{document: document, segments: append([]string(nil), sourceSegments...)}
			if previous, resolved := r.resolvedResourceIDs[resourceURI.String()]; resolved {
				if !sameResourceLocation(previous, location) {
					return nil, r.issue(
						"reference.resource_collision",
						fmt.Sprintf("stable resource URI %q is declared by multiple authored resources", resourceURI.String()),
						document,
						appendPath(sourceSegments, "$id"),
					)
				}
				return map[string]any{"$ref": reusableResourceReference(resourceID, resourceURI)}, nil
			}
			baseURI = resourceURI
		}

		var resolved any
		var diagnostics []Diagnostic
		if reference, ok := typed["$ref"]; ok {
			resolved, diagnostics = r.resolveReferenceObject(typed, reference, document, segments, sourceSegments, baseURI)
		} else {
			resolved, diagnostics = r.resolveObject(typed, document, segments, sourceSegments, baseURI)
		}
		if len(diagnostics) == 0 && r.deduplicateResources && hasResourceID {
			r.resolvedResourceIDs[resourceURI.String()] = resourceLocation{
				document: document,
				segments: append([]string(nil), sourceSegments...),
			}
		}
		return resolved, diagnostics
	case []any:
		resolved := make([]any, len(typed))
		var diagnostics []Diagnostic
		for index, child := range typed {
			var issues []Diagnostic
			resolved[index], issues = r.resolveNode(
				child,
				document,
				appendPath(segments, strconv.Itoa(index)),
				appendPath(sourceSegments, strconv.Itoa(index)),
				baseURI,
			)
			diagnostics = append(diagnostics, issues...)
		}
		return resolved, diagnostics
	default:
		return value, nil
	}
}

func sameResourceLocation(first, second resourceLocation) bool {
	if first.document != second.document || len(first.segments) != len(second.segments) {
		return false
	}
	for index := range first.segments {
		if first.segments[index] != second.segments[index] {
			return false
		}
	}
	return true
}

func (r *referenceResolver) unsupportedReferenceKeywordDiagnostics(value map[string]any, document string, segments []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, keyword := range []string{"$dynamicRef", "$recursiveRef"} {
		if _, exists := value[keyword]; !exists {
			continue
		}
		diagnostics = append(diagnostics, r.singleIssue(
			"reference.unsupported",
			fmt.Sprintf("reference keyword %q is not supported", keyword),
			document,
			appendPath(segments, keyword),
		))
	}
	return diagnostics
}

func stableResourceID(value map[string]any, baseURI *url.URL) (string, *url.URL, bool) {
	identifier, ok := value["$id"].(string)
	if !ok || identifier == "" {
		return "", baseURI, false
	}
	parsed, err := url.Parse(identifier)
	if err != nil {
		return identifier, baseURI, false
	}
	return identifier, baseURI.ResolveReference(parsed), true
}

func reusableResourceReference(identifier string, resourceURI *url.URL) string {
	if resourceURI.IsAbs() {
		return resourceURI.String()
	}
	return identifier
}

func documentBaseURI(document string) *url.URL {
	return &url.URL{Path: "/" + normalizeRepositoryPath(document)}
}

func (r *referenceResolver) resolveObject(value map[string]any, document string, segments, sourceSegments []string, baseURI *url.URL) (map[string]any, []Diagnostic) {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	resolved := make(map[string]any, len(value))
	var diagnostics []Diagnostic
	for _, key := range keys {
		child, issues := r.resolveNode(value[key], document, appendPath(segments, key), appendPath(sourceSegments, key), baseURI)
		resolved[key] = child
		diagnostics = append(diagnostics, issues...)
	}
	return resolved, diagnostics
}

func (r *referenceResolver) resolveReferenceObject(value map[string]any, reference any, document string, segments, sourceSegments []string, baseURI *url.URL) (any, []Diagnostic) {
	resolvedReference, diagnostics := r.resolveReference(reference, document, appendPath(segments, "$ref"), baseURI)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	if len(value) == 1 {
		return resolvedReference, nil
	}

	siblings := make(map[string]any, len(value)-1)
	for key, child := range value {
		if key != "$ref" {
			siblings[key] = child
		}
	}
	resolvedSiblings, diagnostics := r.resolveObject(siblings, document, segments, sourceSegments, baseURI)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	if authoredAllOf, ok := resolvedSiblings["allOf"]; ok {
		resolvedSiblings["allOf"] = []any{resolvedReference, map[string]any{"allOf": authoredAllOf}}
	} else {
		resolvedSiblings["allOf"] = []any{resolvedReference}
	}
	return resolvedSiblings, nil
}

func (r *referenceResolver) resolveReference(value any, referringDocument string, segments []string, baseURI *url.URL) (any, []Diagnostic) {
	reference, ok := value.(string)
	if !ok || reference == "" {
		return nil, r.issue("reference.invalid", "reference must be a non-empty string", referringDocument, segments)
	}
	targetDocument, fragment, issue := r.classifyReference(referringDocument, reference, segments)
	if issue != nil {
		return nil, []Diagnostic{*issue}
	}
	key := targetDocument + "#" + fragment
	if r.active[key] {
		return nil, r.issue("reference.cycle", fmt.Sprintf("reference %q forms a cycle", reference), referringDocument, segments)
	}
	r.active[key] = true
	defer delete(r.active, key)

	target, issue := r.loadTarget(targetDocument, referringDocument, segments)
	if issue != nil {
		return nil, []Diagnostic{*issue}
	}
	selected, err := selectFragment(target, fragment)
	if err != nil {
		return nil, r.issue("reference.fragment", fmt.Sprintf("reference %q has an unresolved fragment", reference), referringDocument, segments)
	}
	parsedReference, _ := url.Parse(reference)
	resolvedBaseURI := baseURI.ResolveReference(parsedReference)
	resolvedBaseURI.Fragment = ""
	return r.resolveNode(selected, targetDocument, nil, fragmentSegments(fragment), resolvedBaseURI)
}

func fragmentSegments(fragment string) []string {
	if fragment == "" {
		return nil
	}
	encoded := strings.Split(strings.TrimPrefix(fragment, "/"), "/")
	segments := make([]string, len(encoded))
	for index, segment := range encoded {
		segments[index] = strings.NewReplacer("~1", "/", "~0", "~").Replace(segment)
	}
	return segments
}

func (r *referenceResolver) classifyReference(referringDocument, reference string, segments []string) (string, string, *Diagnostic) {
	if strings.HasPrefix(reference, "#") {
		return referringDocument, strings.TrimPrefix(reference, "#"), nil
	}
	portable := strings.ReplaceAll(reference, `\`, "/")
	if filepath.IsAbs(filepath.FromSlash(portable)) || filepath.VolumeName(filepath.FromSlash(portable)) != "" || strings.HasPrefix(portable, "/") {
		issue := r.singleIssue("reference.unsupported", fmt.Sprintf("reference %q is not repository-relative", reference), referringDocument, segments)
		return "", "", &issue
	}
	parsed, err := url.Parse(portable)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" {
		issue := r.singleIssue("reference.unsupported", fmt.Sprintf("reference %q is not repository-relative", reference), referringDocument, segments)
		return "", "", &issue
	}
	target := normalizeRepositoryPath(filepath.Join(filepath.Dir(referringDocument), filepath.FromSlash(parsed.Path)))
	candidate := filepath.Join(r.root, filepath.FromSlash(target))
	if !containedBy(r.root, candidate) {
		issue := r.singleIssue("reference.escape", fmt.Sprintf("reference %q escapes the repository root", reference), referringDocument, segments)
		return "", "", &issue
	}
	return target, parsed.Fragment, nil
}

func (r *referenceResolver) loadTarget(targetDocument, referringDocument string, segments []string) (any, *Diagnostic) {
	if value, ok := r.documents[targetDocument]; ok {
		return value, nil
	}
	if r.allowed != nil {
		if _, ok := r.allowed[targetDocument]; !ok {
			issue := r.singleIssue("reference.unsupported", "reference target is not an explicit authored input", referringDocument, segments)
			return nil, &issue
		}
	}
	candidate := filepath.Join(r.root, filepath.FromSlash(targetDocument))
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		code, message := "reference.read", "referenced document could not be read"
		if errors.Is(err, os.ErrNotExist) {
			code, message = "reference.missing", "referenced document does not exist"
		}
		issue := r.singleIssue(code, message, referringDocument, segments)
		return nil, &issue
	}
	if !containedBy(r.root, canonical) {
		issue := r.singleIssue("reference.escape", "reference resolves outside the repository root", referringDocument, segments)
		return nil, &issue
	}
	file, err := os.Open(canonical)
	if err != nil {
		issue := r.singleIssue("reference.read", "referenced document could not be read", referringDocument, segments)
		return nil, &issue
	}
	defer file.Close()
	value, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		issue := r.singleIssue("reference.parse", "referenced document is not valid JSON", referringDocument, segments)
		return nil, &issue
	}
	r.documents[targetDocument] = value
	return value, nil
}

func canonicalRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func containedBy(root, candidate string) bool {
	relative, err := filepath.Rel(root, filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func selectFragment(value any, fragment string) (any, error) {
	if fragment == "" {
		return value, nil
	}
	if !strings.HasPrefix(fragment, "/") {
		return nil, errors.New("fragment is not a JSON Pointer")
	}
	current := value
	for _, encoded := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		segment := strings.NewReplacer("~1", "/", "~0", "~").Replace(encoded)
		switch typed := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = typed[segment]
			if !ok {
				return nil, errors.New("object member does not exist")
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, errors.New("array element does not exist")
			}
			current = typed[index]
		default:
			return nil, errors.New("fragment traverses a scalar")
		}
	}
	return current, nil
}

func normalizeRepositoryPath(value string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(value, `\`, "/"))))
}

func appendPath(segments []string, segment string) []string {
	return append(append([]string(nil), segments...), segment)
}

func (r *referenceResolver) issue(code, message, document string, segments []string) []Diagnostic {
	return []Diagnostic{r.singleIssue(code, message, document, segments)}
}

func (r *referenceResolver) singleIssue(code, message, document string, segments []string) Diagnostic {
	return newDiagnostic(code, instancePath(segments), message, normalizeRepositoryPath(document))
}
