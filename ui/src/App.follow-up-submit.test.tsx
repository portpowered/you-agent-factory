import "@testing-library/jest-dom/vitest";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act } from "react";
import { describe, expect, it } from "vitest";
import { semanticWorkflowDashboardSnapshot } from "./components/dashboard/test-fixtures";
import { APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID } from "./testing/app-shell-session-preflight-test-utils";
import {
  activeWorkLabel,
  submitWorkCardControls,
} from "./testing/app-shell-submit-follow-up-test-utils";
import {
  baselineSnapshot,
  chainRenderAppFetchMock,
  jsonResponse,
  lastFetchCallBody,
  nonPromptTemplateFetchPaths,
  registerAppDashboardTestLifecycle,
  renderApp,
  renderAppWithDashboardShell,
  terminalSnapshot,
  waitForDashboardShell,
} from "./testing/app-shell-test-utils";
import { seedTimelineSnapshot } from "./testing/app-shell-timeline-seed-utils";
import { selectComboboxOption } from "./testing/select-test-helpers";
import { isSessionFactoryRequest } from "./testing/session-factory-mocks";

describe("App follow-up flows", () => {
  registerAppDashboardTestLifecycle();

  describe("dashboard shell", () => {
    it("renders the submit-work card alongside the existing dashboard widgets", async () => {
      await renderAppWithDashboardShell({ snapshot: terminalSnapshot });

      const dashboardGrid = screen.getByRole("region", {
        name: "you-agent-factory bento board",
      });

      expect(
        within(dashboardGrid).getByRole("article", { name: "Submit work" }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Current selection",
        }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", {
          name: "Trace drill-down",
        }),
      ).toBeTruthy();
      expect(
        within(dashboardGrid).getByRole("article", { name: "Factory graph" }),
      ).toBeTruthy();
      expect(
        dashboardGrid.querySelector('[data-bento-card-id="submit-work"]'),
      ).toBeTruthy();
    });

    it("keeps the export toolbar action available alongside the submit-work card", async () => {
      renderApp({
        snapshot: terminalSnapshot,
        seedCurrentFactoryDocument: false,
        fetchOverride: async (path, method) => {
          if (
            method === "GET" &&
            isSessionFactoryRequest(
              path,
              method,
              APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID,
            )
          ) {
            return jsonResponse(
              {
                code: "NOT_FOUND",
                family: "NOT_FOUND",
                message: "Current named factory not found.",
              },
              404,
              "Not Found",
            );
          }

          return undefined;
        },
      });

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
  });

  describe("submit request flows", () => {
    it("submits configured and empty work requests, while preserving failed form state", async () => {
      const { fetchMock } = renderApp({
        snapshot: semanticWorkflowDashboardSnapshot,
      });
      chainRenderAppFetchMock(fetchMock, async (path, method, _input, init) => {
        if (method !== "POST" || !path.endsWith("/work")) {
          return undefined;
        }

        const body = JSON.parse(String(init?.body ?? "{}"));
        if (body.name === "Retry dashboard request") {
          return new Response(
            JSON.stringify({
              code: "BAD_REQUEST",
              message: "work_type_name is required",
            }),
            {
              headers: { "Content-Type": "application/json" },
              status: 400,
              statusText: "Bad Request",
            },
          );
        }

        return new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
          headers: { "Content-Type": "application/json" },
          status: 201,
        });
      });

      await waitForDashboardShell();

      const {
        requestName,
        requestText,
        submissionItemsList,
        submitButton,
        submitWorkScope,
        workType,
      } = submitWorkCardControls();

      expect(within(submissionItemsList).getAllByRole("listitem")).toHaveLength(
        1,
      );
      const user = userEvent.setup();
      await user.click(workType);
      expect(
        within(await screen.findByRole("listbox")).getByRole("option", {
          name: "story",
        }),
      ).toBeTruthy();
      await user.keyboard("{Escape}");
      expect(submitButton.disabled).toBe(false);
      expect(
        submitWorkScope.queryByText(
          "Choose a work type and enter a request name to continue.",
        ),
      ).toBeNull();

      await selectComboboxOption(user, workType, "story");
      fireEvent.change(requestName, {
        target: { value: "Dashboard smoke request" },
      });
      fireEvent.change(requestText, {
        target: { value: "Review the failed dashboard submission smoke." },
      });
      fireEvent.click(submitButton);

      expect(
        await submitWorkScope.findByText(
          "Your request was submitted. Trace ID: trace-submit-story.",
        ),
      ).toBeTruthy();
      expect(
        lastFetchCallBody(
          fetchMock,
          (path, method) => method === "POST" && path.endsWith("/work"),
        ),
      ).toEqual({
        items: [
          {
            text: "Review the failed dashboard submission smoke.",
            type: "text",
          },
        ],
        name: "Dashboard smoke request",
        workTypeName: "story",
      });

      fireEvent.change(requestName, {
        target: { value: "Dashboard empty payload request" },
      });
      const requestsBeforeEmptyPayloadSubmit =
        nonPromptTemplateFetchPaths(fetchMock).length;
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(nonPromptTemplateFetchPaths(fetchMock)).toHaveLength(
          requestsBeforeEmptyPayloadSubmit + 1,
        );
      });
      expect(
        lastFetchCallBody(
          fetchMock,
          (path, method) => method === "POST" && path.endsWith("/work"),
        ),
      ).toEqual({
        items: [],
        name: "Dashboard empty payload request",
        workTypeName: "story",
      });
      expect(
        await submitWorkScope.findByText(
          "Your request was submitted. Trace ID: trace-submit-story.",
        ),
      ).toBeTruthy();
      expect(
        submitWorkScope.queryByText(
          "We couldn't submit your request. Try again in a moment.",
        ),
      ).toBeNull();

      fireEvent.change(requestName, {
        target: { value: "Retry dashboard request" },
      });
      fireEvent.change(requestText, {
        target: {
          value: "Retry the broken submission from the dashboard shell.",
        },
      });
      fireEvent.click(submitButton);

      expect(
        await submitWorkScope.findByText("work_type_name is required"),
      ).toBeTruthy();
      expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
        `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/work`,
        `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/work`,
        `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/work`,
      ]);
      expect(workType).toHaveTextContent("story");
      expect(requestName.value).toBe("Retry dashboard request");
      expect(requestText.value).toBe(
        "Retry the broken submission from the dashboard shell.",
      );
    });
  });

  describe("multimodal submit", () => {
    it("submits a light text-plus-image multimodal request through the dashboard shell", async () => {
      const { fetchMock } = renderApp({
        snapshot: semanticWorkflowDashboardSnapshot,
      });
      chainRenderAppFetchMock(fetchMock, async (path, method) => {
        if (method !== "POST") {
          return undefined;
        }

        if (path.endsWith("/work/staged-files")) {
          return new Response(
            JSON.stringify({
              fileName: "review.png",
              mediaType: "image/png",
              stagedFileRef: "/tmp/staged/review.png",
              url: "file:///tmp/staged/review.png",
            }),
            {
              headers: { "Content-Type": "application/json" },
              status: 201,
            },
          );
        }

        if (path.endsWith("/work")) {
          return new Response(
            JSON.stringify({ traceId: "trace-submit-multimodal" }),
            {
              headers: { "Content-Type": "application/json" },
              status: 201,
            },
          );
        }

        return undefined;
      });

      await waitForDashboardShell();

      const {
        requestName,
        requestText,
        submitButton,
        submitWorkScope,
        workType,
      } = submitWorkCardControls();

      fireEvent.click(
        submitWorkScope.getByRole("button", { name: "Add input" }),
      );
      fireEvent.click(await screen.findByRole("button", { name: "Image" }));

      await selectComboboxOption(userEvent.setup(), workType, "story");
      fireEvent.change(requestName, {
        target: { value: "Dashboard multimodal request" },
      });
      fireEvent.change(requestText, {
        target: { value: "Review the screenshot and summarize the issue." },
      });
      const imageFileInput =
        document.querySelector<HTMLInputElement>('input[type="file"]');
      if (!imageFileInput) {
        throw new Error("expected one submit-work file input");
      }
      fireEvent.change(imageFileInput, {
        target: {
          files: [createStageableFile("png-bytes", "review.png", "image/png")],
        },
      });

      expect(
        await submitWorkScope.findByText("review.png (image/png)"),
      ).toBeTruthy();

      fireEvent.click(submitButton);

      expect(
        await submitWorkScope.findByText(
          "Your request was submitted. Trace ID: trace-submit-multimodal.",
        ),
      ).toBeTruthy();
      expect(
        lastFetchCallBody(
          fetchMock,
          (path, method) => method === "POST" && path.endsWith("/work"),
        ),
      ).toEqual({
        items: [
          {
            text: "Review the screenshot and summarize the issue.",
            type: "text",
          },
          {
            fileName: "review.png",
            mediaType: "image/png",
            stagedFileRef: "/tmp/staged/review.png",
            type: "image",
            url: "file:///tmp/staged/review.png",
          },
        ],
        name: "Dashboard multimodal request",
        workTypeName: "story",
      });
      expect(nonPromptTemplateFetchPaths(fetchMock)).toEqual([
        `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/work/staged-files`,
        `/factory-sessions/${APP_SHELL_RESOLVED_DEFAULT_SESSION_UUID}/work`,
      ]);
    });
  });

  describe("workstation and live totals", () => {
    it("shows workstation-scoped workstation runs on the free-floating cards", async () => {
      renderApp({ snapshot: semanticWorkflowDashboardSnapshot });

      fireEvent.click(
        await screen.findByLabelText("Select Review workstation"),
      );

      const workstationInfo = await screen.findByRole("article", {
        name: "Current selection",
      });
      expect(within(workstationInfo).getByText("Active Story")).toBeTruthy();
      expect(
        within(workstationInfo).queryByText(
          /codex \/ Session ID \/ sess-active-story/,
        ),
      ).toBeNull();

      const expandButton = within(workstationInfo).getByRole("button", {
        name: "Expand",
      });
      fireEvent.click(expandButton);
      await waitFor(() => {
        expect(
          within(workstationInfo).getAllByText(activeWorkLabel).length,
        ).toBeGreaterThan(0);
        expect(
          within(workstationInfo).getByText(
            /codex \/ Session ID \/ sess-active-story/,
          ),
        ).toBeTruthy();
        expect(within(workstationInfo).getByText("Repeated work")).toBeTruthy();
        expect(
          within(workstationInfo).getByText("Raw outcome: REJECTED"),
        ).toBeTruthy();
      });

      fireEvent.click(
        await screen.findByLabelText("Select Implement workstation"),
      );
      const implementInfo = await screen.findByRole("article", {
        name: "Current selection",
      });
      expect(
        within(implementInfo).getByText(
          "No active work is running on this workstation.",
        ),
      ).toBeTruthy();
      fireEvent.click(
        within(implementInfo).getByRole("button", { name: "Expand" }),
      );
      await waitFor(() => {
        expect(within(implementInfo).getByText("Retry Story")).toBeTruthy();
        expect(
          within(implementInfo).getByText("Session log unavailable"),
        ).toBeTruthy();
      });
    });

    it("updates completed and failed totals from the live stream", async () => {
      renderApp({ snapshot: baselineSnapshot });

      await waitForDashboardShell();
      act(() => {
        seedTimelineSnapshot({
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
          within(
            within(workTotals)
              .getByText("Completed")
              .closest("article") as HTMLElement,
          ).getByText("3"),
        ).toBeTruthy();
        expect(
          within(
            within(workTotals)
              .getByText("Failed")
              .closest("article") as HTMLElement,
          ).getByText("1"),
        ).toBeTruthy();
      });
      expect(
        screen.queryByRole("status", { name: /Event stream/i }),
      ).toBeNull();
    });
  });
});

function createStageableFile(
  content: string,
  fileName: string,
  mediaType: string,
): File {
  const file = new File([content], fileName, { type: mediaType });
  Object.defineProperty(file, "arrayBuffer", {
    value: async () => new TextEncoder().encode(content).buffer,
  });
  return file;
}
