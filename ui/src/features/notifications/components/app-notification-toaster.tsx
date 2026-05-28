import { Toaster } from "sonner";

export const GLOBAL_TOAST_DURATION_MS = 3000;

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
