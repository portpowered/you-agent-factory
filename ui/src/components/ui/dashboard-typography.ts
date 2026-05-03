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
export const DASHBOARD_SUPPORTING_LABELS_CLASS = "af-dashboard-supporting-labels";

export type DashboardTypographyRole =
  | "pageHeading"
  | "sectionHeading"
  | "bodyText"
  | "supportingText";

export interface DashboardTypographyContractEntry {
  className: string;
  minimumRem: number;
  replacedLiterals: readonly (typeof DASHBOARD_RETIRED_TEXT_SIZE_LITERALS)[number][];
  role: DashboardTypographyRole;
  usage: readonly string[];
}

// Shared dashboard typography contract for page titles, widget headings, body copy,
// and supporting metadata. Future dashboard cleanup should reuse these role classes
// instead of reintroducing raw `text-[...]` literals on cards, drill-downs, or charts.
export const DASHBOARD_TYPOGRAPHY_CONTRACT: readonly DashboardTypographyContractEntry[] = [
  {
    className: DASHBOARD_PAGE_HEADING_CLASS,
    minimumRem: 1.85,
    replacedLiterals: [],
    role: "pageHeading",
    usage: ["page title"],
  },
  {
    className: DASHBOARD_SECTION_HEADING_CLASS,
    minimumRem: 1.02,
    replacedLiterals: [],
    role: "sectionHeading",
    usage: ["widget title", "detail section heading"],
  },
  {
    className: DASHBOARD_BODY_TEXT_CLASS,
    minimumRem: 0.9,
    replacedLiterals: ["text-[0.78rem]"],
    role: "bodyText",
    usage: ["detail copy", "table body text", "trace metadata"],
  },
  {
    className: DASHBOARD_SUPPORTING_TEXT_CLASS,
    minimumRem: 0.8,
    replacedLiterals: ["text-[0.72rem]", "text-[0.74rem]", "text-[0.68rem]"],
    role: "supportingText",
    usage: ["metadata labels", "chart-axis/supporting labels"],
  },
] as const;

