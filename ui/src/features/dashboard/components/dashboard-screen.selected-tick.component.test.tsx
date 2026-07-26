import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { expect, it } from "vitest";
import { tickZeroInitialStructureRequestEvents } from "../../../testing/app-shell-layout-test-utils";
import {
  baselineSnapshot,
  registerAppDashboardTestLifecycle,
} from "../../../testing/app-shell-test-utils";
import { selectedTickTimelineEvents } from "../../../testing/app-shell-timeline-test-utils";
import { renderDashboardScreenWithShell as renderAppWithDashboardShell } from "./testing/dashboard-screen-test-render";

function getStateNodeByLabel(label: string): HTMLElement {
  const displayLabel = label.split(":").at(-1) ?? label;
  const button = screen.getByLabelText(`Select ${displayLabel} work-state`);
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} work state to be rendered in a React Flow node`,
    );
  }

  return node;
}

registerAppDashboardTestLifecycle();

it("renders a streamed tick-zero initial structure as a ready dashboard", async () => {
  await renderAppWithDashboardShell({
    snapshot: baselineSnapshot,
    timelineEvents: tickZeroInitialStructureRequestEvents,
  });

  expect(screen.queryByText("Loading dashboard")).toBeNull();
  const graphViewport = screen.getByRole("region", {
    name: "Work graph viewport",
  });
  expect(
    within(graphViewport).getByRole("button", { name: "Zoom In" }),
  ).toBeTruthy();
  expect(
    screen.getByRole<HTMLInputElement>("slider", { name: "Timeline tick" })
      .value,
  ).toBe("0");
  expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
});

it("projects totals and selection panels from the selected event tick", async () => {
  await renderAppWithDashboardShell({
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
  expect(
    await screen.findByRole("article", { name: "Current selection" }),
  ).toBeTruthy();
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
    expect(
      within(getStateNodeByLabel("story:new")).getByText(/1 Work$/),
    ).toBeTruthy();
  });
});
