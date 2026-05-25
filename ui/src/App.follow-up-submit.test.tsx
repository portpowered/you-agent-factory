import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { describe, expect, it } from "vitest";
import {
  MockEventSource,
  activeSnapshot,
  baselineSnapshot,
  jsonResponse,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
  terminalSnapshot,
} from "./testing/app-shell-test-utils";
import { DEFAULT_FACTORY_SESSION_ID } from "./api/session-routing";
import {
  activeWorkLabel,
  submitWorkCardControls,
} from "./testing/app-shell-submit-follow-up-test-utils";

describe("App follow-up submit and dashboard shell flows", () => {
  registerAppDashboardTestLifecycle();

  it("renders the submit-work card alongside the existing dashboard widgets", async () => {
    renderApp({ snapshot: terminalSnapshot });

    const dashboardGrid = await screen.findByRole("region", {
      name: "you-agent-factory bento board",
    });

    expect(within(dashboardGrid).getByRole("article", { name: "Submit work" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Current selection" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Trace drill-down" })).toBeTruthy();
    expect(within(dashboardGrid).getByRole("article", { name: "Factory graph" })).toBeTruthy();
    expect(dashboardGrid.querySelector('[data-bento-card-id="submit-work"]')).toBeTruthy();
  });

  it("keeps the export toolbar action available alongside the submit-work card", async () => {
    const { fetchMock } = renderApp({ snapshot: terminalSnapshot });
    fetchMock.mockResolvedValueOnce(
      jsonResponse(
        {
          code: "NOT_FOUND",
          family: "NOT_FOUND",
          message: "Current named factory not found.",
        },
        404,
        "Not Found",
      ),
    );

    await screen.findByRole("button", { name: "Export PNG" });
    fireEvent.click(screen.getByRole("button", { name: "Export PNG" }));

    const exportDialog = await screen.findByRole("dialog", {
      name: "Export factory",
    });
    await waitFor(() => {
      expect(
        within(exportDialog).getByText(
          "The current factory definition is not available yet. Wait for the current-factory API to expose the authored definition before exporting.",
        ),
      ).toBeTruthy();
    });
    expect(within(exportDialog).getByLabelText("Factory name")).toBeTruthy();
  });

  it("submits configured and empty work requests, while preserving failed form state", async () => {
    const { fetchMock } = renderApp({ snapshot: activeSnapshot });
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
          headers: { "Content-Type": "application/json" },
          status: 201,
        }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
          headers: { "Content-Type": "application/json" },
          status: 201,
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            code: "BAD_REQUEST",
            message: "work_type_name is required",
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 400,
            statusText: "Bad Request",
          },
        ),
      );

    await screen.findByRole("heading", { name: "U" });

    const { requestName, requestText, submitButton, submitWorkScope, workType } =
      submitWorkCardControls();

    expect(Array.from(workType.options, (option) => option.value)).toContain("story");
    expect(submitButton.disabled).toBe(true);
    expect(
      submitWorkScope.getByText("Choose a work type and enter a request name to continue."),
    ).toBeTruthy();

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, { target: { value: "Dashboard smoke request" } });
    fireEvent.change(requestText, {
      target: { value: "Review the failed dashboard submission smoke." },
    });
    fireEvent.click(submitButton);

    expect(
      await submitWorkScope.findByText("Your request was submitted. Trace ID: trace-submit-story."),
    ).toBeTruthy();
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      name: "Dashboard smoke request",
      payload: "Review the failed dashboard submission smoke.",
      workTypeName: "story",
    });

    fireEvent.change(requestName, {
      target: { value: "Dashboard empty payload request" },
    });
    fireEvent.click(submitButton);

    expect(
      await submitWorkScope.findByText("Your request was submitted. Trace ID: trace-submit-story."),
    ).toBeTruthy();
    expect(JSON.parse(String(fetchMock.mock.calls.at(-1)?.[1]?.body))).toEqual({
      name: "Dashboard empty payload request",
      payload: "",
      workTypeName: "story",
    });

    fireEvent.change(requestName, { target: { value: "Retry dashboard request" } });
    fireEvent.change(requestText, {
      target: { value: "Retry the broken submission from the dashboard shell." },
    });
    fireEvent.click(submitButton);

    expect(await submitWorkScope.findByText("work_type_name is required")).toBeTruthy();
    expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/work`,
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/work`,
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/work`,
    ]);
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe("Retry the broken submission from the dashboard shell.");
  });

});

describe("App follow-up workstation and live totals flows", () => {
  registerAppDashboardTestLifecycle();

  it("shows workstation-scoped workstation runs on the free-floating cards", async () => {
    renderApp({ snapshot: activeSnapshot });

    fireEvent.click(await screen.findByRole("button", { name: "Select Review workstation" }));

    const workstationInfo = await screen.findByRole("article", {
      name: "Current selection",
    });
    expect(within(workstationInfo).getByText("Active Story")).toBeTruthy();
    expect(
      within(workstationInfo).queryByText(/codex \/ session_id \/ sess-active-story/),
    ).toBeNull();

    const expandButton = within(workstationInfo).getByRole("button", { name: "Expand" });
    fireEvent.click(expandButton);
    await waitFor(() => {
      expect(within(workstationInfo).getAllByText(activeWorkLabel).length).toBeGreaterThan(0);
      expect(within(workstationInfo).getByText(/codex \/ session_id \/ sess-active-story/)).toBeTruthy();
      expect(within(workstationInfo).getByText("Repeated work")).toBeTruthy();
      expect(within(workstationInfo).getByText("Raw outcome: REJECTED")).toBeTruthy();
    });

    fireEvent.click(await screen.findByRole("button", { name: "Select Implement workstation" }));
    const implementInfo = await screen.findByRole("article", { name: "Current selection" });
    expect(within(implementInfo).getByText("No active work is running on this workstation.")).toBeTruthy();
    fireEvent.click(within(implementInfo).getByRole("button", { name: "Expand" }));
    await waitFor(() => {
      expect(within(implementInfo).getByText("Retry Story")).toBeTruthy();
      expect(within(implementInfo).getByText("Session log unavailable")).toBeTruthy();
    });
  });

  it("updates completed and failed totals from the live stream", async () => {
    renderApp({ snapshot: baselineSnapshot });

    await screen.findByRole("heading", { name: "U" });
    const stream = MockEventSource.instances[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.onopen?.(new Event("open"));
      stream.emit("snapshot", {
        ...baselineSnapshot,
        runtime: {
          ...baselineSnapshot.runtime,
          session: {
            ...baselineSnapshot.runtime.session,
            completed_count: 3,
            completed_work_labels: ["work-complete"],
            failed_count: 1,
            failed_work_labels: ["work-failed"],
          },
        },
      });
    });

    await waitFor(() => {
      const workTotals = screen.getByLabelText("work totals");
      expect(
        within(within(workTotals).getByText("Completed").closest("article") as HTMLElement).getByText("3"),
      ).toBeTruthy();
      expect(
        within(within(workTotals).getByText("Failed").closest("article") as HTMLElement).getByText("1"),
      ).toBeTruthy();
      expect(
        screen.getByRole("status", { name: "Event stream live" }),
      ).toBeTruthy();
    });
  });
});
