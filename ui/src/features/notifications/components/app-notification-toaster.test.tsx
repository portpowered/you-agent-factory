import { render, screen } from "@testing-library/react";

import {
  AppNotificationToaster,
  GLOBAL_TOAST_DURATION_MS,
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
  it("configures global sonner notifications as closeable top-right toasts that dismiss after three seconds", () => {
    render(<AppNotificationToaster />);

    const toaster = screen.getByTestId("app-notification-toaster");
    expect(toaster.dataset.closeable).toBe("true");
    expect(toaster.dataset.duration).toBe(String(GLOBAL_TOAST_DURATION_MS));
    expect(toaster.dataset.position).toBe("top-right");
  });
});
