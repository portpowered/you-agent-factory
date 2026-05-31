import { Toaster } from "sonner";

import { GLOBAL_TOAST_DURATION_MS } from "../lib/notification-toast-duration";

export { GLOBAL_TOAST_DURATION_MS };

export function AppNotificationToaster() {
  return (
    <Toaster
      closeButton
      duration={GLOBAL_TOAST_DURATION_MS}
      position="top-right"
      richColors
    />
  );
}
