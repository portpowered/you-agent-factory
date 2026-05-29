import "../../testing/bun-app-shell-module-mocks";
import { screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "bun:test";
import { graphStateSmokeTimelineEvents } from "../components/dashboard/fixtures";
import {
  MockEventSource,
  baselineSnapshot,
  registerAppDashboardTestLifecycle,
  renderApp,
} from "./app-shell-test-utils";

registerAppDashboardTestLifecycle();

describe("App replay shell bun harness", () => {
  it("opens the factory event stream and renders replay shell state", async () => {
    renderApp({
      snapshot: baselineSnapshot,
      timelineEvents: graphStateSmokeTimelineEvents,
    });

    await waitFor(() => {
      expect(MockEventSource.instances.length).toBeGreaterThan(0);
    });
    expect(screen.getByRole("region", { name: "dashboard summary" })).toBeInTheDocument();
  });
});
