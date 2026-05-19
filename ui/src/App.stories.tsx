import { expect, userEvent, waitFor, within } from "storybook/test";

import { App } from "./App";
import {
  semanticWorkflowDashboardSnapshot,
  singleNodeDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "./components/dashboard/test-fixtures";
import {
  activeStoryTrace,
  buttonVisibleStyle,
  currentSelectionCard,
  expectCurrentSelectionCardID,
  expectGraphWorkstation,
  expectNoPageHorizontalOverflow,
  expectWorkOutcomeSeries,
  fillSubmitWorkCard,
  requireValue,
  submitWorkCardControls,
} from "./stories/dashboardStorySupport";

export default {
  title: "Infinite You/Workflow Dashboard",
  component: App,
};

export const SemanticGraphComposition = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expectGraphWorkstation(canvasElement, "Select Review workstation");
    expect(
      (await canvas.findAllByText("dispatch-review-active")).length,
    ).toBeGreaterThan(0);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select Implement workstation",
      }),
    );
    const runHistorySection = within(currentSelectionCard(canvasElement))
      .getByRole("heading", { name: "Run history" })
      .closest("section");
    const resolvedRunHistorySection = requireValue(
      runHistorySection,
      "expected implement workstation run history section",
    );
    await userEvent.click(
      within(resolvedRunHistorySection).getByRole("button", { name: "Expand" }),
    );
    await expect(
      within(resolvedRunHistorySection).getByText("Retry Story"),
    ).toBeVisible();
    await expect(await canvas.findByText("Failed Story")).toBeVisible();
  },
};

export const SingleNodeGraph = {
  parameters: {
    dashboardApi: {
      snapshot: singleNodeDashboardSnapshot,
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectGraphWorkstation(canvasElement, "Select Intake workstation");
  },
};

export const MediumWorkflowGraph = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expectGraphWorkstation(canvasElement, "Select Implement workstation");
    await expect(
      await canvas.findByRole("button", {
        name: "Select Document workstation",
      }),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("button", { name: "Select Review workstation" }),
    ).toBeVisible();
  },
};

export const TwentyNodeWorkflowGraph = {
  parameters: {
    dashboardApi: {
      snapshot: twentyNodeDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const viewport = await canvas.findByRole("region", {
      name: "Work graph viewport",
    });
    const station20 = await expectGraphWorkstation(
      canvasElement,
      "Select Station 20 workstation",
    );

    viewport.scrollLeft = 320;
    viewport.scrollTop = 80;
    await userEvent.pointer([
      {
        keys: "[MouseLeft>]",
        target: viewport,
        coords: { x: 640, y: 280 },
      },
      {
        target: viewport,
        coords: { x: 360, y: 210 },
      },
      {
        keys: "[/MouseLeft]",
        target: viewport,
        coords: { x: 360, y: 210 },
      },
    ]);

    station20.scrollIntoView({ block: "center", inline: "center" });
    const stationRect = station20.getBoundingClientRect();
    const stationCenterX = stationRect.left + stationRect.width / 2;
    const stationCenterY = stationRect.top + stationRect.height / 2;
    const hitTarget = document.elementFromPoint(stationCenterX, stationCenterY);
    expect(station20.contains(hitTarget)).toBe(true);

    await userEvent.click(station20);
    await expect(station20).toHaveAttribute("aria-pressed", "true");
    await expect(
      canvas.getByRole("article", { name: "Current selection" }),
    ).toBeVisible();
  },
};

export const DashboardImprovementsSmoke = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => <App />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    const graphCard = await canvas.findByRole("article", {
      name: "Factory graph",
    });
    const submitWorkCard = await canvas.findByRole("article", {
      name: "Submit work",
    });
    await expect(graphCard).toBeVisible();
    await expect(submitWorkCard).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("combobox", { name: "Work type" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("textbox", { name: "Request name" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("textbox", { name: "Request" }),
    ).toBeVisible();
    await expect(
      within(submitWorkCard).getByRole("button", { name: "Submit work" }),
    ).toBeDisabled();
    await expect(
      await canvas.findByRole("button", { name: "Move Work totals" }),
    ).toBeVisible();
    expect(canvas.queryByRole("button", { name: "Move" })).toBeNull();

    const workTotalsItem = canvasElement.querySelector<HTMLElement>(
      '[data-bento-card-id="work-totals"]',
    );
    expect(
      workTotalsItem?.querySelector(".react-resizable-handle-e"),
    ).not.toBeNull();
    expect(
      workTotalsItem?.querySelector(".react-resizable-handle-s"),
    ).not.toBeNull();
    expect(
      workTotalsItem?.querySelector(".react-resizable-handle-se"),
    ).not.toBeNull();

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select Implement workstation",
      }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Implement"),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", { name: /Active Story/ }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText(
        "work-active-story",
      ),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Select story:implemented state",
      }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Current work"),
    ).toBeVisible();
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Active Story"),
    ).toBeVisible();
    await expect(
      within(currentSelectionCard(canvasElement)).getByText(
        "work-active-story",
      ),
    ).toBeVisible();
    await userEvent.click(
      await canvas.findByRole("button", { name: "Select story:blocked state" }),
    );
    await expect(
      within(currentSelectionCard(canvasElement)).getByText("Current work"),
    ).toBeVisible();
    await expect(
      within(currentSelectionCard(canvasElement)).getByText(
        "No work is recorded for this place at the selected tick.",
      ),
    ).toBeVisible();
    expectCurrentSelectionCardID(canvasElement);

    expect(canvas.queryByRole("article", { name: /Retry|Rework/i })).toBeNull();
    expect(canvas.queryByRole("article", { name: /Timing/i })).toBeNull();

    const outcomeChart = await canvas.findByRole("article", {
      name: "Work outcome chart",
    });
    await expect(outcomeChart).toBeVisible();
    expectWorkOutcomeSeries(outcomeChart);
    await expect(
      within(outcomeChart).getByRole("img", { name: /Work outcome chart/ }),
    ).toBeVisible();
  },
};

export const DashboardImprovementsSmokeNarrow = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <App />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const frame = canvasElement.firstElementChild;

    await expect(
      await canvas.findByRole("article", { name: "Submit work" }),
    ).toBeVisible();
    await userEvent.click(
      (await canvas.findAllByRole("button", { name: /Active Story/ }))[0],
    );

    const dashboardGrid = await canvas.findByRole("region", {
      name: "Infinite You bento board",
    });
    const dashboardScope = within(dashboardGrid);

    await expect(
      dashboardScope.getByRole("article", { name: "Submit work" }),
    ).toBeVisible();
    await expect(
      dashboardScope.getByRole("article", { name: "Current selection" }),
    ).toBeVisible();
    await expect(
      dashboardScope.getByRole("article", { name: "Trace drill-down" }),
    ).toBeVisible();
    expect(frame?.getBoundingClientRect().width ?? 0).toBeLessThanOrEqual(360);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const CurrentSelectionProviderSessionDetailsNarrow = {
  parameters: {
    dashboardApi: {
      snapshot: semanticWorkflowDashboardSnapshot,
      tracesByWorkID: {
        "work-active-story": activeStoryTrace,
      },
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const originalFetch = window.fetch.bind(window);

    window.fetch = async (input, init) => {
      const requestURL =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? input.toString()
            : input.url;
      const requestMethod =
        typeof input === "object" &&
        input !== null &&
        "method" in input &&
        typeof input.method === "string"
          ? input.method
          : init?.method ?? "GET";
      const requestPath = requestURL.startsWith("http")
        ? new URL(requestURL).pathname
        : new URL(requestURL, window.location.origin).pathname;

      if (
        requestMethod.toUpperCase() === "GET" &&
        requestPath === "/provider-sessions/detail"
      ) {
        return new Response(
          JSON.stringify({
            parse: {
              eventCount: 6,
              functionCalls: [
                {
                  arguments: "{\"path\":\"pkg/api\"}",
                  callId: "call_1",
                  name: "list_dir",
                  order: 1,
                  output: "{\"entries\":3}",
                  status: "completed",
                  turnIndex: 1,
                  type: "function_call",
                },
              ],
              lineCount: 7,
              malformedLineCount: 1,
              parseErrors: [
                {
                  lineNumber: 7,
                  message: "unexpected end of JSON input",
                },
              ],
              reasoning: [
                {
                  order: 1,
                  sourceType: "reasoning",
                  summary: "Inspect the provider-session diff before retrying.",
                  text: "Investigate the failed response stream.",
                  turnIndex: 1,
                },
              ],
              tokenUsage: {
                cachedInputTokens: 5,
                inputTokens: 120,
                outputTokens: 45,
                reasoningOutputTokens: 17,
                totalTokens: 182,
              },
              turns: [
                {
                  eventCount: 4,
                  functionCallCount: 1,
                  index: 1,
                  reasoningCount: 1,
                  responseItemCount: 3,
                  startedAt: "2026-05-18T14:10:00Z",
                },
              ],
              unknownEventCount: 1,
              unknownEvents: [
                {
                  lineNumber: 6,
                  payloadType: "mystery_payload",
                  type: "mystery_event",
                },
              ],
            },
            providerSession: {
              id: "sess-active-story",
              kind: "session_id",
              provider: "codex",
            },
            source: {
              modifiedAt: "2026-05-18T14:11:00Z",
              relativePath: "2026/05/18/rollout-sess-active-story.jsonl",
              sizeBytes: 2048,
            },
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
            statusText: "OK",
          },
        );
      }

      return originalFetch(input, init);
    };

    try {
      await userEvent.click(
        (await canvas.findAllByRole("button", { name: /Active Story/ }))[0],
      );

      const currentSelection = within(currentSelectionCard(canvasElement));
      const providerSessionButton = await currentSelection.findByRole("button", {
        name: "Select provider session codex / session_id / sess-active-story for dispatch dispatch-review-active",
      });

      await userEvent.click(providerSessionButton);

      await expect(
        currentSelection.getByRole("heading", {
          name: "Selected session details",
        }),
      ).toBeVisible();
      await expect(
        await currentSelection.findByText(
          "2026/05/18/rollout-sess-active-story.jsonl",
        ),
      ).toBeVisible();
      await expect(await currentSelection.findByText("Turn 1")).toBeVisible();
      await expect(await currentSelection.findByText("list_dir")).toBeVisible();
      await expect(
        await currentSelection.findByText(
          "Inspect the provider-session diff before retrying.",
        ),
      ).toBeVisible();
      await expect(await currentSelection.findByText("182")).toBeVisible();
      await expect(
        await currentSelection.findByText("unexpected end of JSON input"),
      ).toBeVisible();
      expectNoPageHorizontalOverflow(canvasElement);
    } finally {
      window.fetch = originalFetch;
    }
  },
};

export const DashboardSubmitWorkIntegrationSmoke = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: "/work",
          response: {
            body: {
              traceId: "trace-submit-story",
            },
            status: 201,
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <App />,
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const {
      requestField,
      requestNameField,
      scope,
      submitButton,
      workTypeField,
    } = await submitWorkCardControls(canvasElement);
    const disabledSubmitStyle = buttonVisibleStyle(submitButton);

    expect(
      Array.from(
        (workTypeField as HTMLSelectElement).options,
        (option) => option.value,
      ),
    ).toContain("story");
    await expect(submitButton).toBeDisabled();
    await userEvent.type(requestNameField, "Dashboard smoke request");
    await expect(submitButton).toBeDisabled();
    await userEvent.type(
      requestField,
      "Review the failed dashboard submission smoke.",
    );
    await expect(submitButton).toBeDisabled();
    await userEvent.selectOptions(workTypeField, "story");
    await expect(submitButton).toBeEnabled();
    await waitFor(() => {
      expect(buttonVisibleStyle(submitButton)).not.toEqual(disabledSubmitStyle);
    });
    await userEvent.click(submitButton);
    await expect(
      await scope.findByText(
        "Your request was submitted. Trace ID: trace-submit-story.",
      ),
    ).toBeVisible();
    await expect(requestNameField).toHaveValue("");
    await expect(requestField).toHaveValue("");
    await expect(workTypeField).toHaveValue("story");
    await expect(submitButton).toBeEnabled();
  },
};

export const DashboardSubmitWorkRetryableFailure = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: "/work",
          response: {
            body: {
              code: "BAD_REQUEST",
              message: "work_type_name is required",
            },
            status: 400,
            statusText: "Bad Request",
          },
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <App />,
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const requestText = "Retry the broken submission from the dashboard shell.";
    const requestName = "Retry dashboard request";
    const { requestField, requestNameField, scope, workTypeField } =
      await fillSubmitWorkCard(canvasElement, requestName, requestText);

    await userEvent.click(scope.getByRole("button", { name: "Submit work" }));
    await expect(
      await scope.findByText("work_type_name is required"),
    ).toBeVisible();
    await expect(workTypeField).toHaveValue("story");
    await expect(requestNameField).toHaveValue(requestName);
    await expect(requestField).toHaveValue(requestText);
  },
};
