export const layoutFields = new Set([
  "schemaVersion",
  "annotations",
  "nodeEmptyStates",
]);
export const noteFields = new Set([
  "id",
  "kind",
  "position",
  "size",
  "title",
  "body",
  "tone",
]);
export const imageAnnotationFields = new Set([
  "id",
  "kind",
  "position",
  "size",
  "altText",
  "source",
]);
export const emptyStateFields = new Set(["nodeId", "content"]);
export const textContentFields = new Set(["kind", "text"]);
export const imageContentFields = new Set(["kind", "altText", "source"]);
export const imageSourceFields = new Set(["kind", "mediaType", "base64"]);
