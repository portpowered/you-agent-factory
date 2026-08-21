import { render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  App,
  CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH,
  PACKAGED_FACTORIES_PATH,
} from "./App";

const VERTICAL_SCROLL_CLASS_PATTERN =
  /(?:^|\s)(?:overflow-(?:auto|scroll)|overflow-y-(?:auto|scroll))(?:\s|$)/;

vi.mock("./features/dashboard/public/screen", () => ({
  DashboardScreen: () => <div data-testid="dashboard-screen" />,
}));

vi.mock(
  "./features/factory-emulator/components/customer-factory-emulator-demos",
  () => ({
    CustomerFactoryEmulatorDemos: () => (
      <section data-testid="customer-factory-emulator-demos" />
    ),
  }),
);

vi.mock(
  "./features/packaged-factories/components/packaged-factory-inventory",
  () => ({
    PackagedFactoryInventory: () => (
      <section data-testid="packaged-factory-inventory" />
    ),
  }),
);

vi.mock("./features/notifications/components/app-notification-toaster", () => ({
  AppNotificationToaster: () => null,
}));

vi.mock("./i18n", () => ({
  AppLocaleProvider: ({ children }: { children: ReactNode }) => children,
}));

vi.mock("./theme", () => ({
  AppColorPaletteProvider: ({ children }: { children: ReactNode }) => children,
}));

function renderAndExpectPageShellContract(
  pathname: string,
  expectedClassName: string,
) {
  const view = render(<App locationPathname={pathname} />);
  const shell = screen.getByRole("main");

  expect(shell.className).toBe(expectedClassName);
  expect(shell.className).toContain("overflow-x-clip");
  expect(shell.className).not.toContain("overflow-x-hidden");
  expect(shell.className).not.toMatch(VERTICAL_SCROLL_CLASS_PATTERN);

  return view;
}

describe("alternate application page shells", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("clips the customer emulator shell without adding a vertical scroll owner", () => {
    const { unmount } = renderAndExpectPageShellContract(
      CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH,
      "min-h-screen overflow-x-clip bg-surface p-1 md:p-2",
    );
    expect(screen.getByTestId("customer-factory-emulator-demos")).toBeTruthy();
    unmount();
  });

  it("clips the packaged-factory shell without adding a vertical scroll owner", () => {
    const { unmount } = renderAndExpectPageShellContract(
      PACKAGED_FACTORIES_PATH,
      "min-h-screen overflow-x-clip bg-surface p-4 md:p-6",
    );
    expect(screen.getByTestId("packaged-factory-inventory")).toBeTruthy();
    unmount();
  });
});
