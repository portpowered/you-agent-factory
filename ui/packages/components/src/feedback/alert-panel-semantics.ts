import type { AlertPanelTone, AlertPanelVariant } from "./alert-panel";

export type AlertPanelSemanticVariant =
  | "danger"
  | "empty"
  | "error"
  | "info"
  | "loading"
  | "neutral"
  | "success"
  | "warning";

export interface AlertPanelSemanticConfig {
  busy?: boolean;
  role: "alert" | "status";
  statusLabel?: string;
  tone: AlertPanelTone;
  variant: AlertPanelVariant;
}

export const ALERT_PANEL_SEMANTIC_CONFIG: Record<
  AlertPanelSemanticVariant,
  AlertPanelSemanticConfig
> = {
  neutral: {
    role: "status",
    tone: "neutral",
    variant: "default",
  },
  info: {
    role: "status",
    tone: "info",
    variant: "default",
  },
  success: {
    role: "status",
    tone: "success",
    variant: "default",
  },
  warning: {
    role: "alert",
    tone: "warning",
    variant: "default",
  },
  danger: {
    role: "alert",
    tone: "danger",
    variant: "default",
  },
  error: {
    role: "alert",
    statusLabel: "Error",
    tone: "error",
    variant: "default",
  },
  loading: {
    busy: true,
    role: "status",
    statusLabel: "Loading",
    tone: "neutral",
    variant: "loading",
  },
  empty: {
    role: "status",
    statusLabel: "Empty",
    tone: "neutral",
    variant: "empty",
  },
};

export function resolveAlertPanelSemantic(
  semantic: AlertPanelSemanticVariant,
): AlertPanelSemanticConfig {
  return ALERT_PANEL_SEMANTIC_CONFIG[semantic];
}
