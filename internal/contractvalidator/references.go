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
	authoredResourceDocuments          map[string]string
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

type resolutionScope struct {
	baseURI          *url.URL
	resourceRoot     any
	resourceSegments []string
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
		authoredResourceDocuments:          authoredResourceDocuments(root, document, value, allowed),
		active:                             make(map[string]bool),
		allowed:                            allowed,
		deduplicateResources:               deduplicateResources,
		rejectUnsupportedReferenceKeywords: allowed != nil,
		resolvedResourceIDs:                make(map[string]resourceLocation),
	}
	resolved, diagnostics := resolver.resolveNode(value, document, nil, nil, resolutionScope{
		baseURI:      documentBaseURI(document),
		resourceRoot: value,
	})
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

func authoredResourceDocuments(root, primaryDocument string, primaryValue any, allowed map[string]struct{}) map[string]string {
	resources := make(map[string]string)
	if allowed == nil {
		return resources
	}
	paths := make([]string, 0, len(allowed))
	for path := range allowed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		value := primaryValue
		if path != primaryDocument {
			loaded, issue := loadJSON(root, path, "document")
			if issue != nil {
				continue
			}
			value = loaded
		}
		object, ok := value.(map[string]any)
		if !ok {
			continue
		}
		identifier, ok := object["$id"].(string)
		if !ok {
			continue
		}
		parsed, err := url.Parse(identifier)
		if err != nil || !parsed.IsAbs() || parsed.Fragment != "" {
			continue
		}
		if _, exists := resources[parsed.String()]; !exists {
			resources[parsed.String()] = path
		}
	}
	return resources
}

func (r *referenceResolver) resolveNode(value any, document string, segments, sourceSegments []string, scope resolutionScope) (any, []Diagnostic) {
	switch typed := value.(type) {
	case map[string]any:
		if r.rejectUnsupportedReferenceKeywords {
			if diagnostics := r.unsupportedReferenceKeywordDiagnostics(typed, document, segments); len(diagnostics) != 0 {
				return nil, diagnostics
			}
		}
		resourceID, resourceURI, hasResourceID := stableResourceID(typed, scope.baseURI)
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
		}
		if hasResourceID {
			scope = resolutionScope{
				baseURI:          resourceURI,
				resourceRoot:     typed,
				resourceSegments: append([]string(nil), sourceSegments...),
			}
		}

		var resolved any
		var diagnostics []Diagnostic
		if reference, ok := typed["$ref"]; ok {
			resolved, diagnostics = r.resolveReferenceObject(typed, reference, document, segments, sourceSegments, scope)
		} else {
			resolved, diagnostics = r.resolveObject(typed, document, segments, sourceSegments, scope)
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
				scope,
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

func (r *referenceResolver) resolveObject(value map[string]any, document string, segments, sourceSegments []string, scope resolutionScope) (map[string]any, []Diagnostic) {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	resolved := make(map[string]any, len(value))
	var diagnostics []Diagnostic
	for _, key := range keys {
		child, issues := r.resolveNode(value[key], document, appendPath(segments, key), appendPath(sourceSegments, key), scope)
		resolved[key] = child
		diagnostics = append(diagnostics, issues...)
	}
	return resolved, diagnostics
}

func (r *referenceResolver) resolveReferenceObject(value map[string]any, reference any, document string, segments, sourceSegments []string, scope resolutionScope) (any, []Diagnostic) {
	resolvedReference, diagnostics := r.resolveReference(reference, document, appendPath(segments, "$ref"), scope)
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
	resolvedSiblings, diagnostics := r.resolveObject(siblings, document, segments, sourceSegments, scope)
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

func (r *referenceResolver) resolveReference(value any, referringDocument string, segments []string, scope resolutionScope) (any, []Diagnostic) {
	reference, ok := value.(string)
	if !ok || reference == "" {
		return nil, r.issue("reference.invalid", "reference must be a non-empty string", referringDocument, segments)
	}
	targetDocument, fragment, issue := r.classifyReference(referringDocument, reference, segments)
	if issue != nil {
		return nil, []Diagnostic{*issue}
	}

	var target any
	var targetScope resolutionScope
	var sourceSegments []string
	if strings.HasPrefix(reference, "#") {
		target = scope.resourceRoot
		targetScope = scope
		sourceSegments = scope.resourceSegments
	} else {
		target, issue = r.loadTarget(targetDocument, referringDocument, segments)
		if issue != nil {
			return nil, []Diagnostic{*issue}
		}
		parsedReference, _ := url.Parse(reference)
		resolvedBaseURI := scope.baseURI.ResolveReference(parsedReference)
		resolvedBaseURI.Fragment = ""
		targetScope = resolutionScope{
			baseURI:      resolvedBaseURI,
			resourceRoot: target,
		}
	}

	selected, selectedScope, selectedSegments, err := selectFragmentInScope(
		target,
		fragment,
		targetScope,
		sourceSegments,
		!strings.HasPrefix(reference, "#"),
	)
	if err != nil {
		return nil, r.issue("reference.fragment", fmt.Sprintf("reference %q has an unresolved fragment", reference), referringDocument, segments)
	}
	relocationURI, relocatesIDLessResource := referencedResourceRelocationURI(
		selected,
		selectedScope.baseURI,
		referringDocument,
		targetDocument,
		fragment,
	)
	location := resourceLocation{document: targetDocument, segments: append([]string(nil), selectedSegments...)}
	if r.deduplicateResources && relocatesIDLessResource {
		if previous, exists := r.resolvedResourceIDs[relocationURI.String()]; exists {
			if !sameResourceLocation(previous, location) {
				return nil, r.issue(
					"reference.resource_collision",
					fmt.Sprintf("stable resource URI %q is declared by multiple authored resources", relocationURI.String()),
					targetDocument,
					selectedSegments,
				)
			}
			return map[string]any{"$ref": relocationURI.String()}, nil
		}
	}
	key := targetDocument + "#" + instancePath(selectedSegments)
	if r.active[key] {
		return nil, r.issue("reference.cycle", fmt.Sprintf("reference %q forms a cycle", reference), referringDocument, segments)
	}
	r.active[key] = true
	defer delete(r.active, key)

	resolved, diagnostics := r.resolveNode(selected, targetDocument, nil, selectedSegments, selectedScope)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	resolved = preserveReferencedResourceBase(resolved, selected, selectedScope.baseURI, referringDocument, targetDocument, fragment)
	if r.deduplicateResources && relocatesIDLessResource {
		r.resolvedResourceIDs[relocationURI.String()] = location
	}
	return resolved, nil
}

func preserveReferencedResourceBase(resolved, authored any, baseURI *url.URL, referringDocument, document, fragment string) any {
	authoredObject, ok := authored.(map[string]any)
	if !ok {
		return resolved
	}
	identifier, ok := authoredObject["$id"].(string)
	if ok && identifier != "" {
		parsedIdentifier, err := url.Parse(identifier)
		if err != nil || parsedIdentifier.IsAbs() {
			return resolved
		}
		resolvedObject, resolvedIsObject := resolved.(map[string]any)
		if !resolvedIsObject || resolvedObject["$id"] != identifier {
			return resolved
		}
	} else {
		if _, relocates := referencedResourceRelocationURI(authored, baseURI, referringDocument, document, fragment); !relocates {
			return resolved
		}
	}

	return map[string]any{
		"$id":   resourceRelocationURI(baseURI, document, fragment).String(),
		"allOf": []any{resolved},
	}
}

func referencedResourceRelocationURI(authored any, baseURI *url.URL, referringDocument, document, fragment string) (*url.URL, bool) {
	authoredObject, ok := authored.(map[string]any)
	if !ok {
		return baseURI, false
	}
	if identifier, hasIdentifier := authoredObject["$id"].(string); hasIdentifier && identifier != "" {
		return baseURI, false
	}
	if referringDocument == document || !containsOuterBaseDependentResource(authoredObject) {
		return baseURI, false
	}

	return resourceRelocationURI(baseURI, document, fragment), true
}

func resourceRelocationURI(baseURI *url.URL, document, fragment string) *url.URL {
	relocationBase := *baseURI
	query := relocationBase.Query()
	query.Set("you-join-source", normalizeRepositoryPath(document)+"#"+fragment)
	relocationBase.RawQuery = query.Encode()
	return &relocationBase
}

func containsOuterBaseDependentResource(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if identifier, ok := typed["$id"].(string); ok && identifier != "" {
			parsed, err := url.Parse(identifier)
			if err != nil || parsed.IsAbs() {
				return false
			}
			return true
		}
		for _, child := range typed {
			if containsOuterBaseDependentResource(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsOuterBaseDependentResource(child) {
				return true
			}
		}
	}
	return false
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
	if err == nil && parsed.IsAbs() && parsed.RawQuery == "" {
		resource := *parsed
		resource.Fragment = ""
		if target, ok := r.authoredResourceDocuments[resource.String()]; ok {
			return target, parsed.Fragment, nil
		}
	}
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

func selectFragmentInScope(
	value any,
	fragment string,
	scope resolutionScope,
	sourceSegments []string,
	applyRootID bool,
) (any, resolutionScope, []string, error) {
	if fragment == "" {
		return value, scope, sourceSegments, nil
	}
	if !strings.HasPrefix(fragment, "/") {
		return nil, resolutionScope{}, nil, errors.New("fragment is not a JSON Pointer")
	}
	current := value
	currentSegments := append([]string(nil), sourceSegments...)
	applyCurrentID := applyRootID
	for _, encoded := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		if object, ok := current.(map[string]any); ok && applyCurrentID {
			if _, resourceURI, hasResourceID := stableResourceID(object, scope.baseURI); hasResourceID {
				scope = resolutionScope{
					baseURI:          resourceURI,
					resourceRoot:     object,
					resourceSegments: append([]string(nil), currentSegments...),
				}
			}
		}
		segment, err := decodeJSONPointerToken(encoded)
		if err != nil {
			return nil, resolutionScope{}, nil, err
		}
		switch typed := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = typed[segment]
			if !ok {
				return nil, resolutionScope{}, nil, errors.New("object member does not exist")
			}
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, resolutionScope{}, nil, errors.New("array element does not exist")
			}
			current = typed[index]
		default:
			return nil, resolutionScope{}, nil, errors.New("fragment traverses a scalar")
		}
		currentSegments = appendPath(currentSegments, segment)
		applyCurrentID = true
	}
	return current, scope, currentSegments, nil
}

func decodeJSONPointerToken(encoded string) (string, error) {
	for index := 0; index < len(encoded); index++ {
		if encoded[index] != '~' {
			continue
		}
		if index+1 >= len(encoded) || (encoded[index+1] != '0' && encoded[index+1] != '1') {
			return "", errors.New("fragment contains an invalid JSON Pointer escape")
		}
		index++
	}
	return strings.NewReplacer("~1", "/", "~0", "~").Replace(encoded), nil
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
