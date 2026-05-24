import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import {
  baselineSnapshot,
  registerAppDashboardTestLifecycle,
  renderApp,
  terminalSnapshot,
} from "./testing/app-shell-test-utils";
import {
  historicalTimelineSnapshot,
  selectedTickTimelineEvents,
} from "./testing/app-shell-timeline-test-utils";

function getStateNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", { name: `Select ${label} state` });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} state to be rendered in a React Flow node`,
    );
  }

  return node;
}

function expectStateNodeDotCount(label: string, count: number): void {
  const stateNode = getStateNodeByLabel(label);

  expect(
    stateNode.querySelectorAll("[data-state-work-progress-dot]"),
  ).toHaveLength(count);
}

describe("App timeline reconstruction flows", () => {
  registerAppDashboardTestLifecycle();

  it("disables the timeline control until at least two ticks are available", async () => {
    renderApp({ snapshot: historicalTimelineSnapshot });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });

    expect(slider.disabled).toBe(true);
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: "Return to current tick",
      }),
    ).toBeNull();
    expect(screen.queryByText("Current")).toBeNull();
  });

  it("renders a fixed historical tick from the timeline slider", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    expect(slider.value).toBe("4");
    expect(screen.getByText("4/4")).toBeTruthy();
    expect(
      within(screen.getByLabelText("work totals")).getAllByText("1").length,
    ).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Done Story" })).toBeTruthy();

    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(slider.value).toBe("1");
      expect(screen.getByText("1/4")).toBeTruthy();
      expect(screen.queryByRole("button", { name: "Done Story" })).toBeNull();
    });
    expect(screen.queryByText("sess-done-story")).toBeNull();
  });

  it("returns from a fixed timeline tick to the current factory view", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: "Done Story" })).toBeNull();
    });
    expect(screen.queryByText("Current")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Return to current tick" }),
    ).toBeNull();

    fireEvent.change(slider, { target: { value: "4" } });

    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(screen.getByRole("button", { name: "Done Story" })).toBeTruthy();
    });
  });

  it("renders the updated dashboard header formatting through the app shell", async () => {
    renderApp({
      snapshot: terminalSnapshot,
      timelineSnapshots: [historicalTimelineSnapshot, terminalSnapshot],
    });

    const toolbar = await screen.findByRole("region", {
      name: "dashboard summary",
    });
    const slider = within(toolbar).getByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    const languageButton = within(toolbar).getByRole("button", {
      name: "Change language",
    });
    const exportButton = within(toolbar).getByRole("button", {
      name: "Export PNG",
    });
    const streamStatus = within(toolbar).getByRole("status", {
      name: "you-agent-factory event stream connecting",
    });
    const headerControls = Array.from(
      toolbar.querySelectorAll(
        '[aria-label="Timeline tick"], [aria-label="Change language"], [aria-label="Export PNG"], [role="status"]',
      ),
    );

    expect(headerControls).toHaveLength(4);
    expect(headerControls[0]).toBe(streamStatus);
    expect(headerControls[1]).toBe(languageButton);
    expect(headerControls[2]).toBe(slider);
    expect(headerControls[3]).toBe(exportButton);
    expect(within(toolbar).getByText("4/4")).toBeTruthy();
    expect(within(toolbar).queryByText(/Tick \d+ of \d+/)).toBeNull();
    expect(within(toolbar).getByText("Timeline tick").className).toContain(
      "sr-only",
    );
  });

  it("renders totals and selection panels from the selected event tick", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: selectedTickTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    const totals = screen.getByLabelText("work totals");
    expect(slider.value).toBe("4");
    expect(screen.getByText("4/4")).toBeTruthy();
    expect(within(totals).getByText("Completed")).toBeTruthy();
    expect(within(totals).getAllByText("1").length).toBeGreaterThan(0);
    const eventSelection = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(eventSelection).toBeTruthy();
    expect(
      screen.getByRole("article", { name: "Trace drill-down" }),
    ).toBeTruthy();
    expect(screen.queryByText("Trace history unavailable")).toBeNull();

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expect(screen.queryByText("sess-event-story")).toBeNull();
      expect(screen.queryByRole("article", { name: "Event Story" })).toBeNull();
    });
    expect(
      within(totals).getByText("In progress").closest("article")?.textContent,
    ).toContain("1");

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(screen.getByText("2/4")).toBeTruthy();
      expectStateNodeDotCount("story:new", 1);
    });
  });
});
