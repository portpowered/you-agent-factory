import { render, screen } from "@testing-library/react";

import {
  AppNotificationToaster,
  GLOBAL_TOAST_DURATION_MS,
  PERSISTENT_TOAST_DURATION_MS,
  resolveAppNotificationToastDuration,
} from "./app-notification-toaster";

vi.mock("sonner", () => ({
  Toaster: ({
    closeButton,
    duration,
    position,
  }: {
    closeButton?: boolean;
    duration?: number;
    position?: string;
  }) => (
    <div
      data-closeable={String(closeButton)}
      data-duration={String(duration)}
      data-position={position}
      data-testid="app-notification-toaster"
    />
  ),
}));

describe("AppNotificationToaster", () => {
  it("configures closeable top-right toasts with the global success/info TTL default", () => {
    render(<AppNotificationToaster />);

    const toaster = screen.getByTestId("app-notification-toaster");
    expect(toaster.dataset.closeable).toBe("true");
    expect(toaster.dataset.duration).toBe(String(GLOBAL_TOAST_DURATION_MS));
    expect(toaster.dataset.position).toBe("top-right");
  });

  it("documents persistent error duration separate from the global toaster default", () => {
    expect(resolveAppNotificationToastDuration("error")).toBe(
      PERSISTENT_TOAST_DURATION_MS,
    );
    expect(resolveAppNotificationToastDuration("success")).toBe(
      GLOBAL_TOAST_DURATION_MS,
    );
    expect(PERSISTENT_TOAST_DURATION_MS).not.toBe(GLOBAL_TOAST_DURATION_MS);
  });
});
