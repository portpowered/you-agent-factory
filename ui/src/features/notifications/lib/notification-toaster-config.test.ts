import {
  GLOBAL_TOAST_DURATION_MS,
  getAppNotificationToasterProps,
  PERSISTENT_TOAST_DURATION_MS,
  resolveAppNotificationToastDuration,
} from "./notification-toaster-config";

describe("notification toaster config", () => {
  it("uses persistent duration for errors and global TTL for success and info", () => {
    expect(resolveAppNotificationToastDuration("error")).toBe(
      PERSISTENT_TOAST_DURATION_MS,
    );
    expect(resolveAppNotificationToastDuration("success")).toBe(
      GLOBAL_TOAST_DURATION_MS,
    );
    expect(resolveAppNotificationToastDuration("info")).toBe(
      GLOBAL_TOAST_DURATION_MS,
    );
    expect(PERSISTENT_TOAST_DURATION_MS).not.toBe(GLOBAL_TOAST_DURATION_MS);
  });

  it("configures the global toaster with closeable top-right success/info defaults", () => {
    expect(getAppNotificationToasterProps()).toEqual({
      closeButton: true,
      duration: GLOBAL_TOAST_DURATION_MS,
      position: "top-right",
      richColors: true,
    });
  });
});
