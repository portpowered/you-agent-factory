export type LayoutSpacingRole =
  | "tight"
  | "element"
  | "group"
  | "block"
  | "section"
  | "page";

export type LayoutPrimitiveRole =
  | "sectionStack"
  | "cardContent"
  | "pageHeader"
  | "toolbarRow"
  | "formGroup"
  | "dialogBody";

export interface DashboardLayoutContractEntry {
  className: string;
  gapUtility: string;
  primitiveRole: LayoutPrimitiveRole;
  spacingRole: LayoutSpacingRole;
  usage: readonly string[];
}

/** Vertical stack between major page sections or widget groups. */
export const LAYOUT_SECTION_STACK_CLASS = "grid gap-layout-section";

/** Interior stack for card and panel content blocks. */
export const LAYOUT_CARD_CONTENT_STACK_CLASS = "grid gap-layout-element";

/** Page or widget header: title block beside trailing actions. */
export const LAYOUT_PAGE_HEADER_CLASS =
  "flex flex-wrap items-start justify-between gap-layout-group";

/** Dense toolbar / action row aligned with DashboardActionRow rhythm. */
export const LAYOUT_TOOLBAR_ROW_CLASS =
  "flex flex-wrap items-center gap-layout-tight max-md:justify-start";

/** Label, control, and helper copy within a single form field group. */
export const LAYOUT_FORM_GROUP_CLASS = "grid gap-layout-tight";

/** Dialog and modal body regions between header and footer. */
export const LAYOUT_DIALOG_BODY_CLASS = "grid gap-layout-group";

/** Radix dialog content shell (matches shared DialogContent). */
export const LAYOUT_DIALOG_CONTENT_SHELL_CLASS =
  "grid gap-layout-group p-layout-inset-dialog";

/** Card shell interior padding for raised panels. */
export const LAYOUT_CARD_INSET_CLASS =
  "p-layout-inset-card md:p-layout-inset-card-relaxed";

export const DASHBOARD_LAYOUT_CONTRACT: readonly DashboardLayoutContractEntry[] =
  [
    {
      className: LAYOUT_SECTION_STACK_CLASS,
      gapUtility: "gap-layout-section",
      primitiveRole: "sectionStack",
      spacingRole: "section",
      usage: ["page sections", "widget column stacks"],
    },
    {
      className: LAYOUT_CARD_CONTENT_STACK_CLASS,
      gapUtility: "gap-layout-element",
      primitiveRole: "cardContent",
      spacingRole: "element",
      usage: ["card interiors", "definition lists"],
    },
    {
      className: LAYOUT_PAGE_HEADER_CLASS,
      gapUtility: "gap-layout-group",
      primitiveRole: "pageHeader",
      spacingRole: "group",
      usage: ["page titles with actions", "widget headers"],
    },
    {
      className: LAYOUT_TOOLBAR_ROW_CLASS,
      gapUtility: "gap-layout-tight",
      primitiveRole: "toolbarRow",
      spacingRole: "tight",
      usage: ["icon toolbars", "compact action clusters"],
    },
    {
      className: LAYOUT_FORM_GROUP_CLASS,
      gapUtility: "gap-layout-tight",
      primitiveRole: "formGroup",
      spacingRole: "tight",
      usage: ["label + input stacks", "fieldset groups"],
    },
    {
      className: LAYOUT_DIALOG_BODY_CLASS,
      gapUtility: "gap-layout-group",
      primitiveRole: "dialogBody",
      spacingRole: "group",
      usage: ["dialog main content", "mutation dialog bodies"],
    },
  ] as const;
