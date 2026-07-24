/** Stable category path for `@you-agent-factory/components/feedback`. */
export const COMPONENTS_CATEGORY = "feedback" as const;

export type ComponentsCategory = typeof COMPONENTS_CATEGORY;

export type {
  AlertPanelProps,
  AlertPanelSemanticVariant,
  AlertPanelStatusLabelProps,
  AlertPanelTextProps,
  AlertPanelTitleProps,
  AlertPanelTone,
  AlertPanelVariant,
} from "./alert-panel";
export {
  AlertPanel,
  AlertPanelStatusLabel,
  AlertPanelText,
  AlertPanelTitle,
} from "./alert-panel";
export { Skeleton } from "./skeleton";
