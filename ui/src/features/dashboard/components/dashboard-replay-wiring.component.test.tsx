// @component-test-runner vitest: factory-emulator package declarations contain relative imports Bun cannot execute.
import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID } from "../../../testing/app-shell-session-preflight-test-utils";
import {
  MockEventSource,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
} from "../../../testing/app-shell-test-utils";
import {
  historicalTimelineSnapshot,
  selectedTickTimelineEvents,
} from "../../../testing/app-shell-timeline-test-utils";
import { renderDashboardScreen } from "./testing/dashboard-screen-test-render";

async function requireEventStream(): Promise<MockEventSource> {
  return await waitFor(() => {
    const stream = MockEventSource.instances[0];

    if (!stream) {
      throw new Error("expected factory event stream to be opened");
    }

    return stream;
  });
}

describe("dashboard streamed replay wiring", () => {
  registerAppDashboardTestLifecycle();

  it("connects session replay to fixed-tick dashboard rendering", async () => {
    const { fetchMock } = renderDashboardScreen({
      snapshot: historicalTimelineSnapshot,
    });

    const stream = await requireEventStream();
    expect(stream.url).toBe(
      `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );

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
});
