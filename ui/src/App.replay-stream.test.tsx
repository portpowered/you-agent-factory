import "../testing/bun-app-shell-module-mocks";
import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it } from "bun:test";
import {
  failureAnalysisTimelineEvents,
  graphStateSmokeTimelineEvents,
  resourceCountAvailablePlaceID,
  resourceCountBackendWorldViewCountsByTick,
  resourceCountTimelineEvents,
} from "./components/dashboard/fixtures";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import {
  MockEventSource,
  baselineSnapshot,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
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

function getWorkstationNodeByLabel(label: string): HTMLElement {
  const button = screen.getByRole("button", {
    name: `Select ${label} workstation`,
  });
  const node = button.closest(".react-flow__node");

  if (!(node instanceof HTMLElement)) {
    throw new Error(
      `expected ${label} workstation to be rendered in a React Flow node`,
    );
  }

  return node;
}

function expectFixedReviewWorkstationDimensions(): void {
  const reviewNode = getWorkstationNodeByLabel("Review");

  expect(reviewNode.getAttribute("style")).toContain("width: 156px");
  expect(reviewNode.getAttribute("style")).toContain("height: 196px");
}

function expectRenderedResourceCountMatchesBackendWorldView(
  tick: number,
): void {
  const expectedCount =
    resourceCountBackendWorldViewCountsByTick[tick]?.[
      resourceCountAvailablePlaceID
    ] ?? 0;

  const resourceNodeLabel = resourceCountAvailablePlaceID.split(":")[0];

  expect(screen.getByLabelText(resourceNodeLabel)).toBeTruthy();
  expect(
    screen
      .getByLabelText(`${expectedCount} resource tokens`)
      .textContent?.trim(),
  ).toBe(String(expectedCount));
}

function expectSeparatedStateMarkerZones(label: string, count: number): void {
  const stateNode = getStateNodeByLabel(label);
  const labelZone = stateNode.querySelector("[data-state-label-zone]");
  const markerZone = stateNode.querySelector("[data-state-marker-zone]");

  expect(labelZone).toBeTruthy();
  expect(markerZone).toBeTruthy();
  expect(labelZone?.textContent).not.toContain(`${count} active`);
  expect(markerZone?.textContent).not.toContain("story");
  expect(
    markerZone?.querySelectorAll("[data-state-work-progress-dot]"),
  ).toHaveLength(count);
}

function requireEventStream(): MockEventSource {
  const stream = MockEventSource.instances[0];

  if (!stream) {
    throw new Error("expected factory event stream to be opened");
  }

  return stream;
}

describe("App streamed replay rendering flows", () => {
  registerAppDashboardTestLifecycle();

  it("smoke tests /events replay rendering without the removed dashboard snapshot route", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = requireEventStream();
    expect(stream.url).toBe(`/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`);

    act(() => {
      for (const event of selectedTickTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(
        screen.getByRole("article", { name: "Current selection" }),
      ).toBeTruthy();
      expect(
        screen.getByRole("article", { name: "Trace drill-down" }),
      ).toBeTruthy();
    });
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expect(screen.queryByText("sess-event-story")).toBeNull();
      expect(
        within(screen.getByLabelText("work totals"))
          .getByText("In progress")
          .closest("article")?.textContent,
      ).toContain("1");
    });
  });

  it("smoke tests failure analysis from streamed events through fixed-tick rendering", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = requireEventStream();

    act(() => {
      for (const event of failureAnalysisTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(
        screen.getByRole("button", { name: "Blocked Analysis Story" }),
      ).toBeTruthy();
      expect(screen.getAllByText("Failed at Review").length).toBeGreaterThan(0);
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Blocked Analysis Story" }),
    );

    const failedDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(failedDetail).getAllByText("Failure reason").length,
    ).toBeGreaterThan(0);
    expect(
      within(failedDetail).getAllByText("provider_rate_limit").length,
    ).toBeGreaterThan(0);
    expect(
      within(failedDetail).getAllByText("Failure message").length,
    ).toBeGreaterThan(0);
    expect(
      within(failedDetail).getAllByText(
        "Provider rate limit exceeded while generating the analysis.",
      ).length,
    ).toBeTruthy();
    expect(
      within(failedDetail).queryByText(
        "Terminal summaries are reconstructed from retained runtime state.",
      ),
    ).toBeNull();

    fireEvent.click(
      await screen.findByRole("button", { name: "Select story:new state" }),
    );

    const currentPositionDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentPositionDetail).getByText("Current work"),
    ).toBeTruthy();
    expect(
      within(currentPositionDetail).getByText("Queued Analysis Story"),
    ).toBeTruthy();
    expect(
      within(currentPositionDetail).getByText("work-queued-analysis"),
    ).toBeTruthy();
    expect(
      within(currentPositionDetail).queryByText(/^Started at /),
    ).toBeNull();

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expect(
        screen.queryByRole("button", { name: "Blocked Analysis Story" }),
      ).toBeNull();
      expect(screen.queryByText("provider_rate_limit")).toBeNull();
      expect(screen.queryByText("sess-blocked-analysis")).toBeNull();
      expect(screen.getByText("Queued Analysis Story")).toBeTruthy();
    });

    fireEvent.change(slider, { target: { value: "4" } });

    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expect(
        screen.getByRole("button", { name: "Blocked Analysis Story" }),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Blocked Analysis Story" }),
    );

    const fixedFailedDetail = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(
      within(fixedFailedDetail).getAllByText("provider_rate_limit").length,
    ).toBeGreaterThan(0);
    expect(
      within(fixedFailedDetail).getAllByText(
        "Provider rate limit exceeded while generating the analysis.",
      ).length,
    ).toBeGreaterThan(0);
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  });

  it("smoke tests resource counts from streamed events against backend world-view counts", async () => {
    const { fetchMock } = renderApp({ snapshot: historicalTimelineSnapshot });

    const stream = requireEventStream();

    act(() => {
      for (const event of resourceCountTimelineEvents) {
        stream.emit("message", event);
      }
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    await waitFor(() => {
      expect(slider.value).toBe("4");
      expect(screen.getByText("4/4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(4);
    });

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(3);
    });

    fireEvent.change(slider, { target: { value: "1" } });

    await waitFor(() => {
      expect(slider.value).toBe("1");
      expect(screen.getByText("1/4")).toBeTruthy();
      expectRenderedResourceCountMatchesBackendWorldView(1);
    });

    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([]);
  });

  it("smoke tests graph state across event replay, terminal selection, and tick changes", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: graphStateSmokeTimelineEvents,
    });

    const slider = await screen.findByRole<HTMLInputElement>("slider", {
      name: "Timeline tick",
    });
    const dashboardGrid = screen.getByRole("region", {
      name: "you-agent-factory bento board",
    });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("9/9")).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
      expect(
        getStateNodeByLabel("story:done").querySelector(
          "[aria-label='2 active items']",
        ),
      ).toBeTruthy();
      expect(
        getStateNodeByLabel("story:failed").querySelector(
          "[aria-label='1 active item']",
        ),
      ).toBeTruthy();
    });

    fireEvent.click(
      screen.getByRole("button", { name: "Select story:done state" }),
    );

    const completedDetail = await within(dashboardGrid).findByRole("article", {
      name: "Current selection",
    });
    expect(within(completedDetail).getByText("Current work")).toBeTruthy();
    expect(
      within(completedDetail).getByText("Completed Smoke Story One"),
    ).toBeTruthy();
    expect(
      within(completedDetail).getByText("Completed Smoke Story Two"),
    ).toBeTruthy();
    expect(
      within(completedDetail).getByText("work-smoke-complete-one"),
    ).toBeTruthy();
    expect(
      within(completedDetail).getByText("work-smoke-complete-two"),
    ).toBeTruthy();
    expect(within(completedDetail).queryAllByText(/^Started at /)).toHaveLength(
      2,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Select story:failed state" }),
    );

    await waitFor(() => {
      const failedDetail = screen.getByRole("article", {
        name: "Current selection",
      });

      expect(within(failedDetail).getByText("Current work")).toBeTruthy();
      expect(within(failedDetail).getByText("Failed Smoke Story")).toBeTruthy();
      expect(within(failedDetail).getByText("work-smoke-failed")).toBeTruthy();
      expect(within(failedDetail).queryAllByText(/^Started at /)).toHaveLength(
        1,
      );
      expect(
        within(failedDetail).getByText("provider_rate_limit"),
      ).toBeTruthy();
      expect(
        within(failedDetail).queryByText(
          "No current work is occupying this place.",
        ),
      ).toBeNull();
    });

    fireEvent.change(slider, { target: { value: "3" } });

    await waitFor(() => {
      expect(slider.value).toBe("3");
      expect(screen.getByText("3/9")).toBeTruthy();
      expect(
        screen.getByRole("button", { name: /Completed Smoke Story One/ }),
      ).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
    });

    fireEvent.change(slider, { target: { value: "2" } });

    await waitFor(() => {
      expect(slider.value).toBe("2");
      expect(screen.getByText("2/9")).toBeTruthy();
      expectSeparatedStateMarkerZones("story:new", 3);
    });

    fireEvent.change(slider, { target: { value: "9" } });

    await waitFor(() => {
      expect(slider.value).toBe("9");
      expect(screen.getByText("9/9")).toBeTruthy();
      expectFixedReviewWorkstationDimensions();
      expect(
        getStateNodeByLabel("story:done").querySelector(
          "[aria-label='2 active items']",
        ),
      ).toBeTruthy();
    });
  });
});
