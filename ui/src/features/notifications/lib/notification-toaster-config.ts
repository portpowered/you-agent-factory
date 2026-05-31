import type { ToasterProps } from "sonner";

import {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "./notification-toast-duration";

export type AppNotificationToastKind = "error" | "success" | "info";

/**
 * Per-outcome toast duration for dashboard notifications.
 * Errors persist until dismissed; success and info use the global TTL.
 */
export function resolveAppNotificationToastDuration(
  kind: AppNotificationToastKind,
): number {
  return kind === "error"
    ? PERSISTENT_TOAST_DURATION_MS
    : GLOBAL_TOAST_DURATION_MS;
}

/** Shared Sonner Toaster props for the dashboard shell. */
export function getAppNotificationToasterProps(): Pick<
  ToasterProps,
  "closeButton" | "duration" | "position" | "richColors"
> {
  return {
    closeButton: true,
    duration: GLOBAL_TOAST_DURATION_MS,
    position: "top-right",
    richColors: true,
  };
}

export {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
} from "./notification-toast-duration";
