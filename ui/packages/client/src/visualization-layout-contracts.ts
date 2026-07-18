export const FACTORY_VISUALIZATION_LAYOUT_SCHEMA_VERSION =
  "factory-visualization-layout/v1" as const;

export type FactoryVisualizationNoteTone =
  | "neutral"
  | "accent"
  | "info"
  | "success"
  | "warning"
  | "danger";

export interface FactoryVisualizationPosition {
  x: number;
  y: number;
}

export interface FactoryVisualizationSize {
  width: number;
  height: number;
}

export interface FactoryVisualizationEmbeddedImageSource {
  kind: "embedded";
  mediaType: "image/png" | "image/jpeg" | "image/webp";
  base64: string;
}

export interface FactoryVisualizationImageContent {
  kind: "image";
  altText: string;
  source: FactoryVisualizationEmbeddedImageSource;
}

export interface FactoryVisualizationNoteAnnotation {
  id: string;
  kind: "note";
  position: FactoryVisualizationPosition;
  size?: FactoryVisualizationSize;
  title?: string;
  body: string;
  tone?: FactoryVisualizationNoteTone;
}

export interface FactoryVisualizationImageAnnotation
  extends FactoryVisualizationImageContent {
  id: string;
  position: FactoryVisualizationPosition;
  size: FactoryVisualizationSize;
}

export type FactoryVisualizationAnnotation =
  | FactoryVisualizationNoteAnnotation
  | FactoryVisualizationImageAnnotation;

export interface FactoryVisualizationTextEmptyState {
  kind: "text";
  text: string;
}

export interface FactoryVisualizationNodeEmptyState {
  nodeId: string;
  content:
    | FactoryVisualizationTextEmptyState
    | FactoryVisualizationImageContent;
}

/** Presentation-only metadata carried beside, never inside, a canonical Factory. */
export interface FactoryVisualizationLayoutV1 {
  schemaVersion: typeof FACTORY_VISUALIZATION_LAYOUT_SCHEMA_VERSION;
  annotations?: FactoryVisualizationAnnotation[];
  nodeEmptyStates?: FactoryVisualizationNodeEmptyState[];
}

export type FactoryVisualizationLayoutIssueCode =
  | "invalid_type"
  | "invalid_value"
  | "missing_required_field"
  | "unsupported_field"
  | "unsupported_layout_schema_version"
  | "invalid_annotation_kind"
  | "invalid_empty_state_kind"
  | "duplicate_annotation_id";

export interface FactoryVisualizationLayoutIssue {
  category: "structure" | "semantic";
  code: FactoryVisualizationLayoutIssueCode;
  path: readonly (string | number)[];
  message: string;
}

export type SafeParseFactoryVisualizationLayoutResult =
  | { success: true; data: FactoryVisualizationLayoutV1 }
  | { success: false; issues: readonly FactoryVisualizationLayoutIssue[] };
