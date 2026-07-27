import "@testing-library/jest-dom/vitest";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { bunVi as vi } from "../../../testing/bun/vi-compat";
import { customerFactoryEmulatorDemoFixtures } from "../lib/customer-demo-fixtures";
import { CustomerFactoryEmulatorDemos } from "./customer-factory-emulator-demos";

function installLifecycleEnvironment() {
  const observers = new Set<{
    callback: IntersectionObserverCallback;
    targets: Set<Element>;
  }>();
  const motionListeners = new Set<(event: MediaQueryListEvent) => void>();
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      private readonly observer = {
        callback: undefined as unknown as IntersectionObserverCallback,
        targets: new Set<Element>(),
      };
      public constructor(callback: IntersectionObserverCallback) {
        this.observer.callback = callback;
        observers.add(this.observer);
      }
      public disconnect() {
        this.observer.targets.clear();
      }
      public observe(target: Element) {
        this.observer.targets.add(target);
      }
      public unobserve(target: Element) {
        this.observer.targets.delete(target);
      }
      public takeRecords() {
        return [];
      }
      public readonly root = null;
      public readonly rootMargin = "0px";
      public readonly thresholds = [0.15];
    },
  );
  vi.stubGlobal("matchMedia", () => ({
    addEventListener: (
      _type: "change",
      listener: (event: MediaQueryListEvent) => void,
    ) => motionListeners.add(listener),
    addListener: () => undefined,
    dispatchEvent: () => true,
    matches: true,
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    removeEventListener: (
      _type: "change",
      listener: (event: MediaQueryListEvent) => void,
    ) => motionListeners.delete(listener),
    removeListener: () => undefined,
  }));
  return {
    intersect(demoID: string, isIntersecting: boolean) {
      for (const observer of observers) {
        for (const target of observer.targets) {
          if ((target as HTMLElement).dataset.demoId !== demoID) continue;
          observer.callback(
            [{ isIntersecting, target } as IntersectionObserverEntry],
            {} as IntersectionObserver,
          );
        }
      }
    },
    motionListenerCount: () => motionListeners.size,
    observerTargetCount: () =>
      [...observers].reduce((count, { targets }) => count + targets.size, 0),
  };
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("CustomerFactoryEmulatorDemos lifecycle isolation", () => {
  it("disposes a removed host and remounts a fresh isolated instance", async () => {
    const environment = installLifecycleEnvironment();
    const clearTimeout = vi.spyOn(window, "clearTimeout");
    const { rerender } = render(<CustomerFactoryEmulatorDemos locale="en" />);
    const success = await screen.findByRole("article", {
      name: "Straightforward success",
    });
    const failure = screen.getByRole("article", {
      name: "Review, rework, and failure",
    });
    await waitFor(() => expect(environment.observerTargetCount()).toBe(2));
    const mountedMotionListeners = environment.motionListenerCount();
    expect(mountedMotionListeners).toBeGreaterThan(0);

    fireEvent.change(
      within(success).getByRole("combobox", { name: "Playback speed" }),
      { target: { value: "4" } },
    );
    act(() => environment.intersect("repeat-review-failure", true));
    fireEvent.click(within(failure).getByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(
        within(failure).getByText("Playing", { exact: true }),
      ).toBeVisible(),
    );
    const clearedBeforeUnmount = clearTimeout.mock.calls.length;

    rerender(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    await waitFor(() => {
      expect(environment.observerTargetCount()).toBe(1);
      expect(environment.motionListenerCount()).toBe(
        mountedMotionListeners / 2,
      );
      expect(clearTimeout.mock.calls.length).toBeGreaterThan(
        clearedBeforeUnmount,
      );
    });
    expect(
      within(success).getByRole("combobox", { name: "Playback speed" }),
    ).toHaveValue("4");

    rerender(<CustomerFactoryEmulatorDemos locale="en" />);
    const remountedFailure = await screen.findByRole("article", {
      name: "Review, rework, and failure",
    });
    await waitFor(() => {
      expect(environment.observerTargetCount()).toBe(2);
      expect(environment.motionListenerCount()).toBe(mountedMotionListeners);
      expect(
        within(remountedFailure).getByText("Ready", { exact: true }),
      ).toBeVisible();
    });
    expect(
      within(remountedFailure).getByRole("combobox", {
        name: "Playback speed",
      }),
    ).toHaveValue("1");
    expect(
      within(remountedFailure).getByRole("slider", {
        name: "Select replay tick",
      }),
    ).toHaveValue("0");
  });
});
