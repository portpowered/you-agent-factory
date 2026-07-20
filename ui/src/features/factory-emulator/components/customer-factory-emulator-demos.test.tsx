import "@testing-library/jest-dom/vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FactoryEmulatorScenario } from "@you-agent-factory/factory-emulator";
import { afterEach, describe, expect, it, vi } from "vitest";

import { customerFactoryEmulatorDemoFixtures } from "../lib/customer-demo-fixtures";
import { CustomerFactoryEmulatorDemos } from "./customer-factory-emulator-demos";

function installPlaybackEnvironment(initialReducedMotion = false) {
  let intersectionCallback: IntersectionObserverCallback | undefined;
  const motionListeners = new Set<(event: MediaQueryListEvent) => void>();
  let reducedMotion = initialReducedMotion;
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      public constructor(callback: IntersectionObserverCallback) {
        intersectionCallback = callback;
      }
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
    addEventListener: (
      _type: "change",
      listener: (event: MediaQueryListEvent) => void,
    ) => motionListeners.add(listener),
    addListener: () => undefined,
    dispatchEvent: () => true,
    get matches() {
      return reducedMotion;
    },
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    removeEventListener: (
      _type: "change",
      listener: (event: MediaQueryListEvent) => void,
    ) => motionListeners.delete(listener),
    removeListener: () => undefined,
  }));
  return {
    intersect(isIntersecting: boolean) {
      intersectionCallback?.(
        [{ isIntersecting } as IntersectionObserverEntry],
        {} as IntersectionObserver,
      );
    },
    setReducedMotion(matches: boolean) {
      reducedMotion = matches;
      for (const listener of motionListeners) {
        listener({ matches } as MediaQueryListEvent);
      }
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("CustomerFactoryEmulatorDemos playback", () => {
  it("autoplays once in view and preserves manual pause across visibility", async () => {
    const environment = installPlaybackEnvironment();
    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    const demo = screen.getByRole("article", {
      name: "Straightforward success",
    });
    await waitFor(() =>
      expect(within(demo).getByText("1 Work total")).toBeVisible(),
    );

    environment.intersect(true);
    await waitFor(() =>
      expect(within(demo).getByText("Playing", { exact: true })).toBeVisible(),
    );
    environment.intersect(false);
    await waitFor(() =>
      expect(within(demo).getByText("Ready", { exact: true })).toBeVisible(),
    );
    environment.intersect(true);
    await waitFor(() =>
      expect(within(demo).getByText("Playing", { exact: true })).toBeVisible(),
    );

    fireEvent.click(within(demo).getByRole("button", { name: "Pause" }));
    environment.intersect(false);
    environment.intersect(true);
    await waitFor(() =>
      expect(within(demo).getByText("Ready", { exact: true })).toBeVisible(),
    );
  });

  it("requires explicit play whenever reduced motion is active", async () => {
    const environment = installPlaybackEnvironment(true);
    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    const demo = screen.getByRole("article", {
      name: "Straightforward success",
    });
    await waitFor(() =>
      expect(within(demo).getByText("1 Work total")).toBeVisible(),
    );

    environment.intersect(true);
    expect(within(demo).getByText("Ready", { exact: true })).toBeVisible();
    environment.setReducedMotion(false);
    environment.intersect(false);
    environment.intersect(true);
    expect(within(demo).getByText("Ready", { exact: true })).toBeVisible();
    fireEvent.click(within(demo).getByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(within(demo).getByText("Playing", { exact: true })).toBeVisible(),
    );

    environment.setReducedMotion(true);
    await waitFor(() =>
      expect(within(demo).getByText("Ready", { exact: true })).toBeVisible(),
    );
    environment.setReducedMotion(false);
    environment.intersect(false);
    environment.intersect(true);
    expect(within(demo).getByText("Ready", { exact: true })).toBeVisible();
    fireEvent.click(within(demo).getByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(within(demo).getByText("Playing", { exact: true })).toBeVisible(),
    );
  });

  it("does not override step or history intent on first viewport entry", async () => {
    const environment = installPlaybackEnvironment();
    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    const demo = screen.getByRole("article", {
      name: "Straightforward success",
    });
    await waitFor(() =>
      expect(within(demo).getByText("1 Work total")).toBeVisible(),
    );

    fireEvent.click(within(demo).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(demo).getByText(/Preparing the launch summary/),
      ).toBeVisible(),
    );
    fireEvent.change(
      within(demo).getByRole("slider", { name: "Select replay tick" }),
      { target: { value: "0" } },
    );
    environment.intersect(true);
    expect(within(demo).getByText("Viewing history")).toBeVisible();
    expect(
      within(demo).queryByText("Playing", { exact: true }),
    ).not.toBeInTheDocument();
  });
});

describe("CustomerFactoryEmulatorDemos", () => {
  it("renders independent ready demos and exposes live and terminal states", async () => {
    render(<CustomerFactoryEmulatorDemos locale="en" />);

    const success = screen.getByRole("article", {
      name: "Straightforward success",
    });
    const failure = screen.getByRole("article", {
      name: "Review, rework, and failure",
    });
    await waitFor(() =>
      expect(within(success).getByText("1 Work total")).toBeVisible(),
    );
    expect(within(failure).getByText("1 Work total")).toBeVisible();

    fireEvent.click(within(success).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(success).getByText(
          "Execute: Preparing the launch summary (1.5 seconds virtual time)",
        ),
      ).toBeVisible(),
    );
    expect(within(failure).getByText("Ready")).toBeVisible();

    fireEvent.click(within(success).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(success).getByRole("region", {
          name: "Successful completion",
        }),
      ).toBeVisible(),
    );
    expect(within(success).getByText("1 completed")).toBeVisible();
    expect(within(failure).queryByText("1 completed")).not.toBeInTheDocument();

    fireEvent.click(within(success).getByRole("button", { name: "Restart" }));
    await waitFor(() =>
      expect(
        within(success).queryByRole("region", {
          name: "Successful completion",
        }),
      ).not.toBeInTheDocument(),
    );
    expect(within(success).getByText("1 Work total")).toBeVisible();
  });

  it("falls back to canonical workstation copy during history inspection", async () => {
    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    const demo = screen.getByRole("article", {
      name: "Straightforward success",
    });
    await waitFor(() =>
      expect(within(demo).getByText("1 Work total")).toBeVisible(),
    );

    fireEvent.click(within(demo).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(demo).getByText(/Preparing the launch summary/),
      ).toBeVisible(),
    );
    fireEvent.click(within(demo).getByRole("button", { name: "Step" }));
    await waitFor(() =>
      expect(
        within(demo).getByRole("region", { name: "Successful completion" }),
      ).toBeVisible(),
    );

    fireEvent.change(
      within(demo).getByRole("slider", { name: "Select replay tick" }),
      {
        target: { value: "1" },
      },
    );
    await waitFor(() =>
      expect(
        within(demo).getByText(
          "Execute: Working at Execute (1.5 seconds virtual time)",
        ),
      ).toBeVisible(),
    );
    expect(
      within(demo).queryByText(/Preparing the launch summary/),
    ).not.toBeInTheDocument();
    expect(within(demo).getByText("Viewing history")).toBeVisible();
  });
});

describe("CustomerFactoryEmulatorDemos timeline controls", () => {
  it.each(["Play", "Step"])(
    "%s returns from history before advancing the canonical run",
    async (action) => {
      installPlaybackEnvironment(true);
      render(
        <CustomerFactoryEmulatorDemos
          fixtures={[customerFactoryEmulatorDemoFixtures.success]}
          locale="en"
        />,
      );
      const demo = screen.getByRole("article", {
        name: "Straightforward success",
      });
      await waitFor(() =>
        expect(within(demo).getByText("1 Work total")).toBeVisible(),
      );

      fireEvent.click(within(demo).getByRole("button", { name: "Step" }));
      await waitFor(() =>
        expect(
          within(demo).getByText(/Preparing the launch summary/),
        ).toBeVisible(),
      );
      const slider = within(demo).getByRole("slider", {
        name: "Select replay tick",
      });
      fireEvent.change(slider, { target: { value: "0" } });
      expect(within(demo).getByText("Viewing history")).toBeVisible();
      expect(within(demo).getByRole("button", { name: action })).toBeEnabled();

      fireEvent.click(within(demo).getByRole("button", { name: action }));
      await waitFor(() =>
        expect((slider as HTMLInputElement).value).toBe(
          (slider as HTMLInputElement).max,
        ),
      );
      expect(
        within(demo).queryByText("Viewing history"),
      ).not.toBeInTheDocument();
    },
  );

  it("exposes controlled speed and terminal ticks through keyboard controls", async () => {
    const user = userEvent.setup();
    installPlaybackEnvironment(true);
    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[customerFactoryEmulatorDemoFixtures.success]}
        locale="en"
      />,
    );
    const demo = screen.getByRole("article", {
      name: "Straightforward success",
    });
    await waitFor(() =>
      expect(within(demo).getByText("1 Work total")).toBeVisible(),
    );
    const speed = within(demo).getByRole("combobox", {
      name: "Playback speed",
    });
    fireEvent.change(speed, { target: { value: "4" } });
    expect(speed).toHaveValue("4");

    const step = within(demo).getByRole("button", { name: "Step" });
    step.focus();
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(
        within(demo).getByText(/Preparing the launch summary/),
      ).toBeVisible(),
    );
    await user.click(step);
    const terminal = await within(demo).findByRole("region", {
      name: "Successful completion",
    });
    expect(terminal).toBeVisible();
    const slider = within(demo).getByRole("slider", {
      name: "Select replay tick",
    });
    expect((slider as HTMLInputElement).value).toBe(
      (slider as HTMLInputElement).max,
    );
  });
});

describe("CustomerFactoryEmulatorDemos unavailable states", () => {
  it("contains invalid setup to one demo region", async () => {
    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => undefined);
    const invalidScenario = {
      ...customerFactoryEmulatorDemoFixtures.repeatReviewFailure.scenario,
      factory: { name: "wrong-factory" },
    } satisfies FactoryEmulatorScenario;

    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[
          customerFactoryEmulatorDemoFixtures.success,
          {
            ...customerFactoryEmulatorDemoFixtures.repeatReviewFailure,
            scenario: invalidScenario,
          },
        ]}
        locale="en"
      />,
    );

    expect(await screen.findByText("1 Work total")).toBeVisible();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "This demo could not be prepared",
    );
    expect(
      screen.getByRole("article", { name: "Straightforward success" }),
    ).toBeVisible();
    consoleError.mockRestore();
  });

  it("presents bundled fixtures without initial Work as unavailable", async () => {
    const fixture = customerFactoryEmulatorDemoFixtures.success;
    const emptyScenario = {
      ...fixture.scenario,
      initialSubmissions: [],
    } satisfies FactoryEmulatorScenario;

    render(
      <CustomerFactoryEmulatorDemos
        fixtures={[{ ...fixture, scenario: emptyScenario }]}
        locale="en"
      />,
    );

    expect(
      await screen.findByText("No initial Work is available for this demo."),
    ).toBeVisible();
    expect(screen.queryByText(/Loading/)).not.toBeInTheDocument();
  });
});
