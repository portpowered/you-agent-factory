package factoryconfig

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// PortableLayoutValidationError preserves the authored layout field that
// failed raw Factory boundary validation.
type PortableLayoutValidationError struct {
	Path    string
	Message string
}

func (e *PortableLayoutValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// PortableLayoutValidationTarget maps one raw layout failure into the
// canonical Factory validation vocabulary shared by public entrypoints.
func PortableLayoutValidationTarget(err error) (interfaces.ValidationTarget, bool) {
	var layoutErr *PortableLayoutValidationError
	if !errors.As(err, &layoutErr) {
		return interfaces.ValidationTarget{}, false
	}
	path := "factory." + strings.TrimPrefix(layoutErr.Path, "factory.")
	code := interfaces.ValidationCodeLayoutInvalidValue
	if strings.Contains(path, ".position") || strings.Contains(path, ".size") {
		code = interfaces.ValidationCodeLayoutInvalidGeometry
	} else if strings.Contains(layoutErr.Message, "Factory embedded-image budget") {
		code = interfaces.ValidationCodeLayoutImageBudgetExceeded
	}
	subjectID := strings.TrimPrefix(path, "factory.layout.")
	if subjectID == path || subjectID == "" {
		subjectID = "layout"
	}
	return interfaces.ValidationTarget{
		Code: code, Severity: interfaces.ValidationSeverityError, Message: layoutErr.Message,
		Subject: interfaces.ValidationSubject{Type: interfaces.ValidationSubjectTypeFactory, ID: subjectID, Location: interfaces.ValidationSubjectLocationDefinition},
		Path:    path,
	}, true
}

// ValidatePortableLayoutBoundaryJSON validates inert layout metadata directly
// from its authored JSON representation.
func ValidatePortableLayoutBoundaryJSON(data []byte) error {
	err := validatePortableLayoutBoundaryJSON(data)
	if err == nil {
		return nil
	}
	message := err.Error()
	return &PortableLayoutValidationError{Path: portableLayoutValidationPath(message), Message: message}
}

func portableLayoutValidationPath(message string) string {
	fields := strings.Fields(message)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "layout") {
		return "layout"
	}
	return strings.TrimRight(fields[0], ":")
}

const (
	factoryLayoutNoteTitleMaxCharacters       = 160
	factoryLayoutNoteBodyMaxCharacters        = 4000
	factoryLayoutImageAlternativeTextMaxRunes = 500
	factoryLayoutEmbeddedImageMaxBytes        = 2 * 1024 * 1024
	factoryLayoutEmbeddedImageTotalMaxBytes   = 8 * 1024 * 1024
	factoryLayoutEmbeddedImageMaxBase64Chars  = 4 * ((factoryLayoutEmbeddedImageMaxBytes + 2) / 3)
	factoryLayoutAnnotationPositionMaxUnits   = 100000
	factoryLayoutAnnotationSizeMaxUnits       = 10000
)

func requireNonBlankString(parent map[string]any, key string, path string) error {
	if err := requireString(parent, key, path); err != nil {
		return err
	}
	if strings.TrimSpace(parent[key].(string)) == "" {
		return fmt.Errorf("%s.%s must contain a non-whitespace character", path, key)
	}
	return nil
}

func requiredStringValue(parent map[string]any, key string, path string) (string, error) {
	if err := requireString(parent, key, path); err != nil {
		return "", err
	}
	return parent[key].(string), nil
}

func requiredObject(parent map[string]any, key string, path string) (map[string]any, error) {
	value, ok := parent[key]
	if !ok || value == nil {
		return nil, fmt.Errorf("%s.%s is required", path, key)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s must be an object", path, key)
	}
	return object, nil
}

func validateLayoutAnnotationArray(parent map[string]any, key string, path string, totalImageBytes *int) error {
	values, ok, err := optionalObjectArray(parent, key, path)
	if !ok || err != nil {
		return err
	}
	seenIDs := make(map[string]struct{}, len(values))
	for index, annotation := range values {
		annotationPath := fmt.Sprintf("%s.%s[%d]", path, key, index)
		if err := validateLayoutAnnotationFields(annotation, annotationPath); err != nil {
			return err
		}
		if err := requireNonBlankString(annotation, "id", annotationPath); err != nil {
			return err
		}
		annotationID := annotation["id"].(string)
		if _, duplicate := seenIDs[annotationID]; duplicate {
			return fmt.Errorf("%s.id %q duplicates an earlier layout annotation", annotationPath, annotationID)
		}
		seenIDs[annotationID] = struct{}{}
		if err := validateLayoutAnnotationPosition(annotation, annotationPath); err != nil {
			return err
		}
		imageBytes, err := validateLayoutAnnotationContent(annotation, annotationPath)
		if err != nil {
			return err
		}
		*totalImageBytes += imageBytes
		if *totalImageBytes > factoryLayoutEmbeddedImageTotalMaxBytes {
			return fmt.Errorf("%s.image.source.data exceeds the %d-byte Factory embedded-image budget", annotationPath, factoryLayoutEmbeddedImageTotalMaxBytes)
		}
	}
	return nil
}

func validateLayoutAnnotationContent(annotation map[string]any, path string) (int, error) {
	kind, err := requiredStringValue(annotation, "kind", path)
	if err != nil {
		return 0, err
	}
	switch kind {
	case "NOTE":
		if err := rejectLayoutAnnotationField(annotation, "image", path); err != nil {
			return 0, err
		}
		if err := validateLayoutAnnotationSize(annotation, path, false); err != nil {
			return 0, err
		}
		return 0, validateLayoutAnnotationNote(annotation, path)
	case "IMAGE":
		if err := rejectLayoutAnnotationField(annotation, "note", path); err != nil {
			return 0, err
		}
		if err := validateLayoutAnnotationSize(annotation, path, true); err != nil {
			return 0, err
		}
		return validateLayoutAnnotationImage(annotation, path)
	default:
		return 0, fmt.Errorf("%s.kind must be one of NOTE, IMAGE", path)
	}
}

func validateLayoutAnnotationFields(annotation map[string]any, path string) error {
	return validateAllowedLayoutFields(annotation, path, "id", "kind", "position", "size", "note", "image")
}

func validateAllowedLayoutFields(value map[string]any, path string, allowedFields ...string) error {
	allowed := make(map[string]struct{}, len(allowedFields))
	for _, field := range allowedFields {
		allowed[field] = struct{}{}
	}
	for field := range value {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("%s.%s is not allowed", path, field)
		}
	}
	return nil
}

func rejectLayoutAnnotationField(annotation map[string]any, field, path string) error {
	if _, ok := annotation[field]; ok {
		return fmt.Errorf("%s.%s is not valid for this annotation kind", path, field)
	}
	return nil
}

func validateLayoutAnnotationPosition(annotation map[string]any, path string) error {
	position, err := requiredObject(annotation, "position", path)
	if err != nil {
		return err
	}
	if err := validateAllowedLayoutFields(position, path+".position", "x", "y"); err != nil {
		return err
	}
	return validateLayoutAnnotationNumbers(position, path+".position", "x", "y", false)
}

func validateLayoutAnnotationSize(annotation map[string]any, path string, required bool) error {
	value, ok := annotation["size"]
	if !ok || value == nil {
		if required {
			return fmt.Errorf("%s.size is required", path)
		}
		return nil
	}
	size, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.size must be an object", path)
	}
	if err := validateAllowedLayoutFields(size, path+".size", "width", "height"); err != nil {
		return err
	}
	return validateLayoutAnnotationNumbers(size, path+".size", "width", "height", true)
}

func validateLayoutAnnotationNumbers(values map[string]any, path, firstKey, secondKey string, positive bool) error {
	maximum := float64(factoryLayoutAnnotationPositionMaxUnits)
	if positive {
		maximum = float64(factoryLayoutAnnotationSizeMaxUnits)
	}
	for _, key := range []string{firstKey, secondKey} {
		value, err := requiredFiniteNumber(values, key, path)
		if err != nil {
			return err
		}
		if positive && value <= 0 {
			return fmt.Errorf("%s.%s must be greater than zero", path, key)
		}
		if (!positive && math.Abs(value) > maximum) || (positive && value > maximum) {
			return fmt.Errorf("%s.%s must be within %s canvas units", path, key, formatLayoutAnnotationMaximum(maximum, positive))
		}
	}
	return nil
}

func requiredFiniteNumber(parent map[string]any, key, path string) (float64, error) {
	if err := requireNumber(parent, key, path); err != nil {
		return 0, err
	}
	value := parent[key].(float64)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s.%s must be finite", path, key)
	}
	return value, nil
}

func formatLayoutAnnotationMaximum(maximum float64, positive bool) string {
	if positive {
		return fmt.Sprintf("greater than zero and no greater than %.0f", maximum)
	}
	return fmt.Sprintf("between -%.0f and %.0f", maximum, maximum)
}

func validateLayoutAnnotationNote(annotation map[string]any, path string) error {
	note, err := requiredObject(annotation, "note", path)
	if err != nil {
		return err
	}
	if err := validateAllowedLayoutFields(note, path+".note", "title", "body", "tone"); err != nil {
		return err
	}
	if err := requireString(note, "body", path+".note"); err != nil {
		return err
	}
	if err := validateLayoutLiteralText(note["body"].(string), path+".note.body", 1, factoryLayoutNoteBodyMaxCharacters); err != nil {
		return err
	}
	if err := requireString(note, "tone", path+".note"); err != nil {
		return err
	}
	switch note["tone"].(string) {
	case "NEUTRAL", "ACCENT", "INFO", "SUCCESS", "WARNING", "DANGER":
	default:
		return fmt.Errorf("%s.note.tone must be one of NEUTRAL, ACCENT, INFO, SUCCESS, WARNING, DANGER", path)
	}
	if title, ok := note["title"]; ok && title != nil {
		titleValue, ok := title.(string)
		if !ok {
			return fmt.Errorf("%s.note.title must be a string", path)
		}
		if err := validateLayoutLiteralText(titleValue, path+".note.title", 0, factoryLayoutNoteTitleMaxCharacters); err != nil {
			return err
		}
	}
	return nil
}

func validateLayoutAnnotationImage(annotation map[string]any, path string) (int, error) {
	image, err := requiredObject(annotation, "image", path)
	if err != nil {
		return 0, err
	}
	return validateLayoutImage(image, path+".image")
}

func validateLayoutImage(image map[string]any, path string) (int, error) {
	if err := validateAllowedLayoutFields(image, path, "source", "alternativeText"); err != nil {
		return 0, err
	}
	if err := requireString(image, "alternativeText", path); err != nil {
		return 0, err
	}
	if err := validateLayoutLiteralText(image["alternativeText"].(string), path+".alternativeText", 1, factoryLayoutImageAlternativeTextMaxRunes); err != nil {
		return 0, err
	}
	source, err := requiredObject(image, "source", path)
	if err != nil {
		return 0, err
	}
	if err := validateAllowedLayoutFields(source, path+".source", "kind", "mediaType", "data"); err != nil {
		return 0, err
	}
	for _, key := range []string{"kind", "mediaType", "data"} {
		if err := requireString(source, key, path+".source"); err != nil {
			return 0, err
		}
	}
	if source["kind"].(string) != "EMBEDDED" {
		return 0, fmt.Errorf("%s.source.kind must be EMBEDDED", path)
	}
	switch source["mediaType"].(string) {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return 0, fmt.Errorf("%s.source.mediaType must be image/png, image/jpeg, or image/webp", path)
	}
	return decodeStrictFactoryLayoutImageData(source["data"].(string), path+".source.data")
}

func validateLayoutLiteralText(value string, path string, minimumCharacters, maximumCharacters int) error {
	characterCount := utf8.RuneCountInString(value)
	if characterCount < minimumCharacters || (minimumCharacters > 0 && strings.TrimSpace(value) == "") {
		return fmt.Errorf("%s must contain at least %d character", path, minimumCharacters)
	}
	if characterCount > maximumCharacters {
		return fmt.Errorf("%s must contain no more than %d characters", path, maximumCharacters)
	}
	return nil
}

func decodeStrictFactoryLayoutImageData(data string, path string) (int, error) {
	if len(data) > factoryLayoutEmbeddedImageMaxBase64Chars {
		return 0, fmt.Errorf("%s exceeds the %d-byte embedded-image limit", path, factoryLayoutEmbeddedImageMaxBytes)
	}
	if !isStrictPaddedBase64(data) {
		return 0, fmt.Errorf("%s must be non-empty strict padded base64", path)
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(data)
	if err != nil {
		return 0, fmt.Errorf("%s must be strict base64: %w", path, err)
	}
	if len(decoded) > factoryLayoutEmbeddedImageMaxBytes {
		return 0, fmt.Errorf("%s exceeds the %d-byte embedded-image limit", path, factoryLayoutEmbeddedImageMaxBytes)
	}
	return len(decoded), nil
}

func isStrictPaddedBase64(value string) bool {
	if len(value) == 0 || len(value)%4 != 0 {
		return false
	}
	firstPadding := strings.IndexByte(value, '=')
	if firstPadding < 0 {
		return onlyBase64Alphabet(value)
	}
	if firstPadding < len(value)-2 || !onlyBase64Padding(value[firstPadding:]) {
		return false
	}
	return onlyBase64Alphabet(value[:firstPadding])
}

func onlyBase64Alphabet(value string) bool {
	for index := range value {
		if !isBase64AlphabetCharacter(value[index]) {
			return false
		}
	}
	return true
}

func isBase64AlphabetCharacter(character byte) bool {
	return character >= 'A' && character <= 'Z' ||
		character >= 'a' && character <= 'z' ||
		character >= '0' && character <= '9' ||
		character == '+' || character == '/'
}

func onlyBase64Padding(value string) bool {
	if len(value) > 2 {
		return false
	}
	for index := range value {
		if value[index] != '=' {
			return false
		}
	}
	return true
}

func nodeHasTextEmptyState(node map[string]any) bool {
	emptyState, ok := node["emptyState"].(map[string]any)
	if !ok {
		return false
	}
	_, hasText := emptyState["text"]
	return hasText
}

func validateLayoutNodeEmptyState(node map[string]any, nodePath string) (int, error) {
	value, ok := node["emptyState"]
	if !ok || value == nil {
		return 0, nil
	}
	emptyState, ok := value.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("%s.emptyState must be an object", nodePath)
	}
	for field := range emptyState {
		if field != "text" && field != "image" {
			return 0, fmt.Errorf("%s.emptyState.%s is not allowed", nodePath, field)
		}
	}
	text, hasText := emptyState["text"]
	image, hasImage := emptyState["image"]
	if (hasText && text == nil) || (hasImage && image == nil) {
		return 0, fmt.Errorf("%s.emptyState must contain exactly one of text or image", nodePath)
	}
	if hasText == hasImage {
		return 0, fmt.Errorf("%s.emptyState must contain exactly one of text or image", nodePath)
	}
	if hasText {
		literalText, ok := text.(string)
		if !ok {
			return 0, fmt.Errorf("%s.emptyState.text must be a string", nodePath)
		}
		return 0, validateLayoutLiteralText(literalText, nodePath+".emptyState.text", 1, factoryLayoutImageAlternativeTextMaxRunes)
	}
	emptyImage, ok := image.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("%s.emptyState.image must be an object", nodePath)
	}
	return validateLayoutImage(emptyImage, nodePath+".emptyState.image")
}

func factoryLayoutAnnotationsAPIFromInternal(annotations []interfaces.FactoryLayoutAnnotationConfig) *[]factoryapi.FactoryLayoutAnnotation {
	if len(annotations) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryLayoutAnnotation, len(annotations))
	for i, annotation := range annotations {
		values[i] = factoryapi.FactoryLayoutAnnotation{
			Id:       annotation.ID,
			Kind:     factoryapi.FactoryLayoutAnnotationKind(annotation.Kind),
			Position: factoryLayoutAnnotationPositionAPIFromInternal(annotation.Position),
			Size:     factoryLayoutAnnotationSizeAPIFromInternal(annotation.Size),
			Note:     factoryLayoutNoteAPIFromInternal(annotation.Note),
			Image:    factoryLayoutImageAPIFromInternal(annotation.Image),
		}
	}
	return &values
}

func factoryLayoutAnnotationPositionAPIFromInternal(position interfaces.FactoryLayoutPointConfig) factoryapi.FactoryLayoutAnnotationPosition {
	return factoryapi.FactoryLayoutAnnotationPosition{X: float32(position.X), Y: float32(position.Y)}
}

func factoryLayoutAnnotationSizeAPIFromInternal(size *interfaces.FactoryLayoutSizeConfig) *factoryapi.FactoryLayoutAnnotationSize {
	if size == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutAnnotationSize{Width: float32(size.Width), Height: float32(size.Height)}
}

func factoryLayoutNoteAPIFromInternal(note *interfaces.FactoryLayoutNoteConfig) *factoryapi.FactoryLayoutNote {
	if note == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutNote{
		Title: stringPtrIfNotEmpty(note.Title),
		Body:  note.Body,
		Tone:  factoryapi.FactoryLayoutNoteTone(note.Tone),
	}
}

func factoryLayoutImageAPIFromInternal(image *interfaces.FactoryLayoutImageConfig) *factoryapi.FactoryLayoutImage {
	if image == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutImage{
		Source: factoryapi.FactoryLayoutImageSource{
			Kind:      factoryapi.FactoryLayoutImageSourceKind(image.Source.Kind),
			MediaType: factoryapi.FactoryLayoutImageSourceMediaType(image.Source.MediaType),
			Data:      decodeFactoryLayoutImageData(image.Source.Data),
		},
		AlternativeText: image.AlternativeText,
	}
}

func decodeFactoryLayoutImageData(data string) []byte {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}
	return decoded
}

func factoryLayoutEmptyStateAPIFromInternal(emptyState *interfaces.FactoryLayoutEmptyStateConfig) *factoryapi.FactoryLayoutEmptyState {
	if emptyState == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutEmptyState{
		Text:  stringPtrIfNotEmpty(emptyState.Text),
		Image: factoryLayoutImageAPIFromInternal(emptyState.Image),
	}
}
