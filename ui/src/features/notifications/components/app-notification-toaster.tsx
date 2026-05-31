import { Toaster } from "sonner";

import {
  GLOBAL_TOAST_DURATION_MS,
  getAppNotificationToasterProps,
  PERSISTENT_TOAST_DURATION_MS,
  resolveAppNotificationToastDuration,
} from "../lib/notification-toaster-config";

export {
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
  resolveAppNotificationToastDuration,
};

export function AppNotificationToaster() {
  return <Toaster {...getAppNotificationToasterProps()} />;
}
