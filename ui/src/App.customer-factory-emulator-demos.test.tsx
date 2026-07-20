import "@testing-library/jest-dom/vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { App, CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH } from "./App";

function installMotionSafePlaybackEnvironment() {
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      public disconnect() {}
      public observe() {}
      public unobserve() {}
      public takeRecords() {
        return [];
      }
      public readonly root = null;
      public readonly rootMargin = "0px";
      public readonly thresholds = [0.15];
    },
  );
  vi.stubGlobal("matchMedia", () => ({
    addEventListener: () => undefined,
    addListener: () => undefined,
    dispatchEvent: () => true,
    matches: true,
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    removeEventListener: () => undefined,
    removeListener: () => undefined,
  }));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("customer Factory emulator website route", () => {
  it("renders both independently labeled demos on the shipped app surface", async () => {
    installMotionSafePlaybackEnvironment();

    await act(async () => {
      render(
        <App
          initialLocale="en"
          locationPathname={CUSTOMER_FACTORY_EMULATOR_DEMOS_PATH}
        />,
      );
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const demos = screen.getByRole("region", {
      name: "Customer Factory emulator demos",
    });
    const success = within(demos).getByRole("article", {
      name: "Straightforward success",
    });
    const failure = within(demos).getByRole("article", {
      name: "Review, rework, and failure",
    });

    await waitFor(() => {
      expect(within(success).getByText("1 Work total")).toBeVisible();
      expect(within(failure).getByText("1 Work total")).toBeVisible();
    });
    expect(success).toHaveAttribute("data-demo-id", "success");
    expect(failure).toHaveAttribute("data-demo-id", "repeat-review-failure");
    expect(screen.queryByRole("main")).toContainElement(demos);
  });
});
