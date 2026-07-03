/** Stable category path for `@you-agent-factory/components/feedback`. */
export const COMPONENTS_CATEGORY = "feedback" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export { AlertPanel, AlertPanelText } from "./alert-panel";
export type {
  AlertPanelProps,
  AlertPanelTextProps,
  AlertPanelTone,
  AlertPanelVariant,
} from "./alert-panel";
export { Skeleton } from "./skeleton";
