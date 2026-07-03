export const DASHBOARD_RETIRED_TEXT_SIZE_LITERALS = [
  "text-[0.78rem]",
  "text-[0.72rem]",
  "text-[0.74rem]",
  "text-[0.68rem]",
] as const;

export const DASHBOARD_PAGE_HEADING_CLASS = "af-dashboard-page-heading";
export const DASHBOARD_SECTION_HEADING_CLASS = "af-dashboard-section-heading";
export const DASHBOARD_BODY_TEXT_CLASS = "af-dashboard-body-text";
export const DASHBOARD_SUPPORTING_TEXT_CLASS = "af-dashboard-supporting-text";
export const DASHBOARD_SUPPORTING_LABEL_CLASS = "af-dashboard-supporting-label";
export const DASHBOARD_BODY_CODE_CLASS = "af-dashboard-body-code";
export const DASHBOARD_SUPPORTING_CODE_CLASS = "af-dashboard-supporting-code";
export const DASHBOARD_WIDGET_SUBTITLE_CLASS = "af-dashboard-widget-subtitle";

/** Material 3 typography families (see typography-role-tokens.css). */
export type MaterialTypographyFamily =
  | "display"
  | "headline"
  | "title"
  | "body"
  | "label";

/** Product extension beside Material families — monospace / code surfaces. */
export type ProductTypographyExtension = "code";

export type MaterialTypographyVariant = "large" | "medium" | "small";

export type MaterialTextColorRole =
  | "on-surface"
  | "on-surface-variant"
  | "on-surface-subtle"
  | "on-surface-disabled"
  | "on-inverse"
  | "code"
  | "on-primary-container"
  | "on-secondary-container"
  | "on-tertiary-container"
  | "on-success-container"
  | "on-warning-container"
  | "on-error-container"
  | "on-info-container";

export type DashboardTypographyRole =
  | "pageHeading"
  | "sectionHeading"
  | "bodyText"
  | "supportingText";

export interface DashboardTypographyContractEntry {
  className: string;
  materialFamily: MaterialTypographyFamily | ProductTypographyExtension;
  materialVariant: MaterialTypographyVariant;
  minimumRem: number;
  replacedLiterals: readonly (typeof DASHBOARD_RETIRED_TEXT_SIZE_LITERALS)[number][];
  role: DashboardTypographyRole;
  textColorRole: MaterialTextColorRole;
  typeUtilityClass: string;
  usage: readonly string[];
}

// Shared dashboard typography contract for page titles, widget headings, body copy,
// and supporting metadata. Prefer these classes (backed by Material scale utilities)
// instead of reintroducing raw `text-[...]` literals on cards, drill-downs, or charts.
export const DASHBOARD_TYPOGRAPHY_CONTRACT: readonly DashboardTypographyContractEntry[] =
  [
    {
      className: DASHBOARD_PAGE_HEADING_CLASS,
      materialFamily: "display",
      materialVariant: "medium",
      minimumRem: 1.85,
      replacedLiterals: [],
      role: "pageHeading",
      textColorRole: "on-surface",
      typeUtilityClass: "type-display-medium",
      usage: ["page title"],
    },
    {
      className: DASHBOARD_SECTION_HEADING_CLASS,
      materialFamily: "title",
      materialVariant: "large",
      minimumRem: 1.02,
      replacedLiterals: [],
      role: "sectionHeading",
      textColorRole: "on-surface",
      typeUtilityClass: "type-title-large",
      usage: ["widget title", "detail section heading"],
    },
    {
      className: DASHBOARD_BODY_TEXT_CLASS,
      materialFamily: "body",
      materialVariant: "medium",
      minimumRem: 0.9,
      replacedLiterals: ["text-[0.78rem]"],
      role: "bodyText",
      textColorRole: "on-surface-variant",
      typeUtilityClass: "type-body-medium",
      usage: ["detail copy", "table body text", "trace metadata"],
    },
    {
      className: DASHBOARD_SUPPORTING_TEXT_CLASS,
      materialFamily: "body",
      materialVariant: "small",
      minimumRem: 0.8,
      replacedLiterals: ["text-[0.72rem]", "text-[0.74rem]", "text-[0.68rem]"],
      role: "supportingText",
      textColorRole: "on-surface-variant",
      typeUtilityClass: "type-body-small",
      usage: ["metadata labels", "chart-axis/supporting labels"],
    },
  ] as const;

export const DASHBOARD_EXTENDED_TYPOGRAPHY_ROLES = [
  {
    className: DASHBOARD_SUPPORTING_LABEL_CLASS,
    materialFamily: "label" as const,
    materialVariant: "medium" as const,
    textColorRole: "on-surface-subtle" as const,
    typeUtilityClass: "type-label-medium",
    usage: ["uppercase field labels", "table column headers"],
  },
  {
    className: DASHBOARD_BODY_CODE_CLASS,
    materialFamily: "code" as const,
    materialVariant: "medium" as const,
    textColorRole: "code" as const,
    typeUtilityClass: "type-code-medium",
    usage: ["transcript blocks", "JSON payloads"],
  },
  {
    className: DASHBOARD_SUPPORTING_CODE_CLASS,
    materialFamily: "code" as const,
    materialVariant: "small" as const,
    textColorRole: "code" as const,
    typeUtilityClass: "type-code-small",
    usage: ["inline metadata code", "compact traces"],
  },
  {
    className: DASHBOARD_WIDGET_SUBTITLE_CLASS,
    materialFamily: "display" as const,
    materialVariant: "small" as const,
    textColorRole: "on-surface-variant" as const,
    typeUtilityClass: "type-display-small",
    usage: ["widget metric subtitles"],
  },
] as const;
