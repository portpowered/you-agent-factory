import { expect, userEvent, waitFor, within } from "storybook/test";
import { App } from "./App";
import type { FactoryValue } from "./api/named-factory";
import {
  singleNodeDashboardSnapshot,
  semanticWorkflowDashboardSnapshot,
  twentyNodeDashboardSnapshot,
} from "./components/dashboard/test-fixtures";
import { formatTimeOfDay } from "./components/ui/formatters";
import { DashboardScreen } from "./features/dashboard";
import { AppLocaleProvider, useAppLocale } from "./i18n";
import {
  activeStoryTrace,
  buttonVisibleStyle,
  currentSelectionCard,
  expectWorkOutcomeSeries,
  expectCurrentSelectionCardID,
  expectGraphWorkstation,
  expectNoPageHorizontalOverflow,
  fillSubmitWorkCard,
  historicalWorkOutcomeSnapshot,
  liveWorkOutcomeSnapshot,
  requireValue,
  submitWorkCardControls,
} from "./stories/dashboardStorySupport";

const editableConfigurationFactoryDefinition =
  buildEditableConfigurationFactoryDefinition();
const editableConfigurationDocument = buildEditableConfigurationDocument();
const promptTemplateContractResponse = {
  availableVariables: [
    {
      category: "ROOT",
      description: "The current work item identifier.",
      example: "{{ .WorkID }}",
      path: ".WorkID",
    },
    {
      category: "INPUT",
      description: "Payload for the first authored input.",
      example: "{{ (index .Inputs 0).Payload }}",
      path: ".Inputs[0].Payload",
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [
    {
      example: "{{ (index .Inputs 1).Payload }}",
      path: ".Inputs[1].Payload",
      reason: "Only input 0 is available for this workstation.",
    },
  ],
};

function buildEditableConfigurationFactoryDefinition(
  overrides: { prompt?: string; workerName?: string } = {},
): FactoryValue {
  return {
    name: "Current Factory",
    workers: [
      {
        model: "gpt-5.5",
        name: "reviewer",
        type: "MODEL_WORKER",
      },
      {
        model: "gpt-5.6",
        name: "planner",
        type: "MODEL_WORKER",
      },
    ],
    workTypes: [],
    workstations: [
      {
        body:
          overrides.prompt ??
          "Review the latest story changes before approval.",
        id: "review",
        inputs: [{ state: "queued", workType: "story" }],
        name: "Review",
        outputs: [{ state: "approved", workType: "story" }],
        promptFile: "prompts/review.md",
        worker: overrides.workerName ?? "reviewer",
      },
    ],
  };
}

function buildEditableConfigurationDocument(
  factoryDefinition: FactoryValue = editableConfigurationFactoryDefinition,
) {
  return {
    factoryDefinition,
    version: {
      logical: 7,
      physical: "2026-05-20T10:00:00Z",
    },
  };
}

function submittedFactoryDefinitionBody(init?: RequestInit): FactoryValue {
  if (typeof init?.body !== "string") {
    return buildEditableConfigurationFactoryDefinition({
      prompt: "Browser verified prompt update.",
      workerName: "planner",
    });
  }

  const requestBody = JSON.parse(init.body) as {
    factoryDefinition?: FactoryValue;
  };

  return (
    requestBody.factoryDefinition ??
    buildEditableConfigurationFactoryDefinition({
      prompt: "Browser verified prompt update.",
      workerName: "planner",
    })
  );
}

function promptTemplateValidationResponse(init?: RequestInit) {
  if (typeof init?.body !== "string") {
    return {
      diagnostics: [],
      valid: true,
    };
  }

  const payload = JSON.parse(init.body) as { prompt?: string };
  if (payload.prompt === "Use {{ .WorkID }}.") {
    return {
      diagnostics: [],
      valid: true,
    };
  }

  return {
    diagnostics: [
      {
        endOffset: 33,
        kind: "UNAVAILABLE_VARIABLE",
        message: "Only input 0 is available.",
        path: ".Inputs[1]",
        sourceText: "(index .Inputs 1)",
        startOffset: 7,
      },
    ],
    valid: false,
  };
}

function validPromptTemplateValidationResponse() {
  return {
    diagnostics: [],
    valid: true,
  };
}

async function delayedValidPromptTemplateValidationMock() {
  await new Promise<void>((resolve) => {
    window.setTimeout(resolve, 25);
  });

  return {
    body: validPromptTemplateValidationResponse(),
    status: 200,
  };
}

function editableConfigurationSection(
  currentSelection: HTMLElement,
): HTMLElement {
  const section = within(currentSelection)
    .getByRole("heading", { name: "Configuration" })
    .closest("section");

  if (!(section instanceof HTMLElement)) {
    throw new Error("expected editable configuration section");
  }

  return section;
}

async function expectSingleEditableConfigurationSection(
  currentSelection: HTMLElement,
): Promise<HTMLElement> {
  await expect(
    within(currentSelection).getAllByRole("heading", {
      name: "Configuration",
    }),
  ).toHaveLength(1);

  return editableConfigurationSection(currentSelection);
}

async function expectEditableConfigurationBrowserFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);

  for (const workstationName of ["Plan", "Implement", "Review"]) {
    await userEvent.click(
      await canvas.findByRole("button", {
        name: `Select ${workstationName} workstation`,
      }),
    );

    const currentSelection = currentSelectionCard(canvasElement);
    await expectSingleEditableConfigurationSection(currentSelection);
    await expect(
      within(currentSelection).getByText(workstationName, {
        selector: "p",
      }),
    ).toBeVisible();
  }

  const currentSelection = currentSelectionCard(canvasElement);
  const section =
    await expectSingleEditableConfigurationSection(currentSelection);
  const sectionScope = within(section);
  const expandButton = sectionScope.getByRole("button", {
    name: "Expand editable configuration",
  });

  await expect(expandButton).toHaveAttribute("aria-expanded", "false");
  expect(sectionScope.queryByLabelText("Worker")).toBeNull();
  expect(sectionScope.queryByLabelText("Prompt")).toBeNull();

  expandButton.focus();
  await userEvent.keyboard("{Enter}");

  const workerField = await sectionScope.findByRole("combobox", {
    name: "Worker",
  });
  const promptField = await sectionScope.findByRole("textbox", {
    name: "Prompt",
  });

  await expect(expandButton).toHaveAttribute("aria-expanded", "true");
  await expect(workerField).toHaveValue("reviewer");
  await expect(promptField).toHaveValue(
    "Review the latest story changes before approval.",
  );
  await expect(expandButton).toHaveAttribute(
    "aria-controls",
    expect.stringContaining("-content"),
  );
  expect(sectionScope.queryByLabelText("Model")).toBeNull();
  expect(sectionScope.queryByLabelText("Template")).toBeNull();

  await userEvent.selectOptions(workerField, "planner");
  await userEvent.clear(promptField);
  await userEvent.click(promptField);
  await userEvent.paste("Browser verified prompt update.");

  await expect(workerField).toHaveValue("planner");
  await expect(promptField).toHaveValue("Browser verified prompt update.");

  const currentSelectionScope = within(currentSelection);
  const saveButton = currentSelectionScope.getByRole("button", {
    name: "Save changes",
  });

  await waitFor(() => {
    expect(
      sectionScope.queryByText(
        "Validating prompt variables for the current draft.",
      ),
    ).toBeNull();
    expect(saveButton).toBeEnabled();
  });
  await expect(workerField).toHaveValue("planner");
  await expect(promptField).toHaveValue("Browser verified prompt update.");

  await userEvent.click(
    await canvas.findByRole("button", {
      name: "Select Plan workstation",
    }),
  );

  const reboundCurrentSelection = currentSelectionCard(canvasElement);
  const reboundSection = await expectSingleEditableConfigurationSection(
    reboundCurrentSelection,
  );
  const reboundScope = within(reboundSection);
  const reboundExpandButton = reboundScope.getByRole("button", {
    name: "Expand editable configuration",
  });

  await expect(
    within(reboundCurrentSelection).getByText("Plan", { selector: "p" }),
  ).toBeVisible();
  await expect(
    within(reboundCurrentSelection).getAllByRole("heading", {
      name: "Configuration",
    }),
  ).toHaveLength(1);

  await userEvent.click(reboundExpandButton);

  await expect(
    await reboundScope.findByText(
      "This running factory definition does not expose editable worker and prompt values for the selected workstation.",
    ),
  ).toBeVisible();
  await expect(
    within(reboundCurrentSelection).getByRole("button", {
      name: "Save changes",
    }),
  ).toBeDisabled();
}

async function expectFactoryGraphHeaderBrowserFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);
  const graphCard = await canvas.findByRole("article", {
    name: "Factory graph",
  });
  const graphHeader = graphCard.querySelector("header");

  if (!(graphHeader instanceof HTMLElement)) {
    throw new Error("expected factory graph card header");
  }

  const headerScope = within(graphHeader);
  await expect(headerScope.getByText("Observe mode")).toBeVisible();
  await userEvent.click(
    headerScope.getByRole("button", {
      name: "Enter factory graph editor",
    }),
  );
  await expect(headerScope.getByText("Editor mode active")).toBeVisible();
  await expect(
    headerScope.getByRole("button", {
      name: "Leave factory graph editor",
    }),
  ).toBeVisible();
  await expect(
    within(graphCard).getByRole("region", {
      name: "Factory graph editor tools",
    }),
  ).toBeVisible();
  await userEvent.click(
    headerScope.getByRole("button", {
      name: "Leave factory graph editor",
    }),
  );
  await expect(headerScope.getByText("Observe mode")).toBeVisible();
}

async function expectPromptHintBrowserFlow(
  canvasElement: HTMLElement,
): Promise<void> {
  const canvas = within(canvasElement);

  await userEvent.click(
    await canvas.findByRole("button", { name: "Select Review workstation" }),
  );

  const currentSelection = currentSelectionCard(canvasElement);
  const section = editableConfigurationSection(currentSelection);
  const sectionScope = within(section);

  await userEvent.click(
    sectionScope.getByRole("button", {
      name: "Expand editable configuration",
    }),
  );

  const promptField = await sectionScope.findByRole("textbox", {
    name: "Prompt",
  });
  const saveButton = within(currentSelection).getByRole("button", {
    name: "Save changes",
  });

  await expect(
    await sectionScope.findByText(
      "Autocomplete is ready with 2 variables for 1 authored input.",
    ),
  ).toBeVisible();
  await expect(
    sectionScope.getByText(
      "Type inside {{ ... }} to see suggestions, or open Monaco completion manually anywhere in the prompt editor.",
    ),
  ).toBeVisible();
  await expect(
    sectionScope.queryByRole("button", {
      name: "Open prompt variable help",
    }),
  ).toBeNull();
  await expect(sectionScope.queryByText("Prompt variable help")).toBeNull();

  promptField.focus();
  await userEvent.clear(promptField);
  await userEvent.type(promptField, "Use {{ (index .Inputs 1).Payload }}.");

  await expect(
    await sectionScope.findByText("Prompt diagnostics"),
  ).toBeVisible();
  await expect(sectionScope.getByText(".Inputs[1]")).toBeVisible();
  await expect(sectionScope.getAllByText("(index .Inputs 1)")[0]).toBeVisible();
  await expect(saveButton).toBeDisabled();
}

function LocalePropagationStory() {
  return (
    <AppLocaleProvider initialLocale="en">
      <LocalePropagationControls />
      <div style={{ maxWidth: "100%", width: "1280px" }}>
        <DashboardScreen />
      </div>
    </AppLocaleProvider>
  );
}

function LocalePropagationControls() {
  const { locale, setLocale } = useAppLocale();

  return (
    <fieldset style={{ display: "flex", gap: "0.75rem", marginBottom: "1rem" }}>
      <legend>Locale verification controls</legend>
      <span>Current locale: {locale}</span>
      <button onClick={() => setLocale("en")} type="button">
        Switch to English
      </button>
      <button onClick={() => setLocale("zh-CN")} type="button">
        Switch to zh-CN
      </button>
    </fieldset>
  );
}

export default {
  title: "you-agent-factory/Workflow Dashboard",
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
    const graphCard = await canvas.findByRole("article", {
      name: "Factory graph",
    });

    await expectGraphWorkstation(canvasElement, "Select Review workstation");
    expect(canvas.queryByText("Operator View")).toBeNull();
    expect(
      within(graphCard).queryByRole("heading", { name: "Current activity" }),
    ).toBeNull();
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
    const failedStoryButton = await canvas.findByRole("button", {
      name: "Failed Story",
    });
    await expect(failedStoryButton).toBeVisible();
    await expect(within(failedStoryButton).getByText("Failed at Repair")).toBeVisible();
    expect(within(failedStoryButton).queryByText(/session_id/i)).toBeNull();
    expect(within(failedStoryButton).queryByText(/codex/i)).toBeNull();
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
    expect(within(graphCard).queryByText("Operator View")).toBeNull();
    expect(
      within(graphCard).queryByRole("heading", { name: "Current activity" }),
    ).toBeNull();
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
    expect(
      within(submitWorkCard).queryByText(
        "Ready to submit. Request details are optional.",
      ),
    ).toBeNull();
    expect(
      within(submitWorkCard).queryByText(
        "Optional. Leave this blank to submit an empty request.",
      ),
    ).toBeNull();
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
    const currentSelection = within(currentSelectionCard(canvasElement));
    const summaryDetails = currentSelection.getByText("Count").closest("dl");
    await expect(
      currentSelection.getByText("Current work"),
    ).toBeVisible();
    await expect(currentSelection.getByText("story: implemented")).toBeVisible();
    await expect(currentSelection.getByText("Active Story")).toBeVisible();
    await expect(
      currentSelection.getByText(
        `Started at ${formatTimeOfDay("2026-04-08T12:00:01Z")}`,
      ),
    ).toBeVisible();
    expect(summaryDetails).not.toBeNull();
    expect(within(summaryDetails ?? canvasElement).queryByText("Work type")).toBeNull();
    expect(within(summaryDetails ?? canvasElement).queryByText("State")).toBeNull();
    expect(within(summaryDetails ?? canvasElement).queryByText("State node ID")).toBeNull();
    expect(currentSelection.queryByText("trace-active-story")).toBeNull();
    const traceDrilldownCard = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });
    expect(
      within(traceDrilldownCard).queryByText(
        "Resolves from selected-tick factory event history.",
      ),
    ).toBeNull();
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
      name: "you-agent-factory bento board",
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

export const CurrentSelectionEditableConfigurationDesktopVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factories/~default/factory/~current/editable-definition",
          response: {
            body: editableConfigurationDocument,
          },
        },
        {
          method: "POST",
          path: "/factory/~current/workstations/Review/prompt-template-validation",
          response: delayedValidPromptTemplateValidationMock,
        },
        {
          method: "POST",
          path: "/factory",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: submittedFactoryDefinitionBody(init),
            status: 201,
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectFactoryGraphHeaderBrowserFlow(canvasElement);
    await expectEditableConfigurationBrowserFlow(canvasElement);
  },
};

export const CurrentSelectionEditableConfigurationNarrowVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factories/~default/factory/~current/editable-definition",
          response: {
            body: editableConfigurationDocument,
          },
        },
        {
          method: "POST",
          path: "/factory/~current/workstations/Review/prompt-template-validation",
          response: delayedValidPromptTemplateValidationMock,
        },
        {
          method: "POST",
          path: "/factory",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: submittedFactoryDefinitionBody(init),
            status: 201,
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "360px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectFactoryGraphHeaderBrowserFlow(canvasElement);
    await expectEditableConfigurationBrowserFlow(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const CurrentSelectionPromptHintVerification = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factories/~default/factory/~current/editable-definition",
          response: {
            body: editableConfigurationDocument,
          },
        },
        {
          method: "GET",
          path: "/factory/~current/workstations/Review/prompt-template-contract",
          response: {
            body: promptTemplateContractResponse,
          },
        },
        {
          method: "POST",
          path: "/factory/~current/workstations/Review/prompt-template-validation",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: promptTemplateValidationResponse(init),
            status: 200,
          }),
        },
        {
          method: "POST",
          path: "/factory",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: submittedFactoryDefinitionBody(init),
            status: 201,
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App />
    </div>
  ),
  tags: ["test"],
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectPromptHintBrowserFlow(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};

export const DashboardSubmitWorkIntegrationSmoke = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: "/factories/~default/work",
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
    await expect(submitButton).toBeDisabled();
    await waitFor(() => {
      expect(buttonVisibleStyle(submitButton)).toEqual(disabledSubmitStyle);
    });
  },
};

export const DashboardSubmitWorkRetryableFailure = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "POST",
          path: "/factories/~default/work",
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

export const HeaderLocalizationVerification = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      timelineSnapshots: [
        historicalWorkOutcomeSnapshot,
        liveWorkOutcomeSnapshot,
      ],
    },
  },
  render: () => (
    <div style={{ maxWidth: "100%", width: "1280px" }}>
      <App initialLocale="en" />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const englishToolbar = await canvas.findByRole("region", {
      name: "dashboard summary",
    });
    const languageButton = within(englishToolbar).getByRole("button", {
      name: "Change language",
    });

    await expect(
      within(englishToolbar).getByRole("heading", { name: "you-agent-factory" }),
    ).toBeVisible();
    await expect(
      within(englishToolbar).getByRole("button", { name: "Export PNG" }),
    ).toBeVisible();
    await expect(await canvas.findByText("5/5")).toBeVisible();

    languageButton.focus();
    await expect(languageButton).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await userEvent.click(
      within(canvasElement.ownerDocument.body).getByRole("menuitemradio", {
        name: "简体中文",
      }),
    );

    const localizedToolbar = await canvas.findByRole("region", {
      name: "仪表板概览",
    });
    await expect(
      within(localizedToolbar).getByRole("button", { name: "切换语言" }),
    ).toBeVisible();
    await expect(
      within(localizedToolbar).getByRole("slider", { name: "时间线刻度" }),
    ).toBeVisible();
    await expect(await canvas.findByText("5/5")).toBeVisible();
    await expect(
      within(localizedToolbar).getByRole("status", {
        name: /you-agent-factory 事件流(正在连接|在线)/,
      }),
    ).toBeVisible();
    await expect(
      within(localizedToolbar).queryByRole("button", { name: "返回当前刻度" }),
    ).toBeNull();
    await expect(
      within(localizedToolbar).getByRole("button", { name: "导出 PNG" }),
    ).toBeVisible();

    await userEvent.click(
      within(localizedToolbar).getByRole("button", { name: "导出 PNG" }),
    );
    const dialog = await within(canvasElement.ownerDocument.body).findByRole("dialog", {
      name: "导出工厂",
    });
    await expect(
      within(dialog).getByRole("button", { name: "取消" }),
    ).toBeVisible();
    await expect(
      within(dialog).getByRole("button", { name: "导出 PNG" }),
    ).toBeVisible();
    await userEvent.click(within(dialog).getByRole("button", { name: "取消" }));
    const localizedLanguageButton = within(localizedToolbar).getByRole("button", {
      name: "切换语言",
    });
    localizedLanguageButton.focus();
    await expect(localizedLanguageButton).toHaveFocus();
    await userEvent.click(localizedLanguageButton);
    await userEvent.click(
      within(canvasElement.ownerDocument.body).getByRole("menuitemradio", {
        name: "English",
      }),
    );
    const restoredToolbar = await canvas.findByRole("region", { name: "dashboard summary" });
    await expect(
      within(restoredToolbar).getByRole("button", { name: "Change language" }),
    ).toBeVisible();
    await expect(await canvas.findByText("5/5")).toBeVisible();
    await expect(
      within(restoredToolbar).getByRole("button", { name: "Export PNG" }),
    ).toBeVisible();
  },
};
export const LocalePropagationVerification = {
  tags: ["test"],
  parameters: {
    dashboardApi: {
      timelineSnapshots: [historicalWorkOutcomeSnapshot, liveWorkOutcomeSnapshot],
    },
  },
  render: () => <LocalePropagationStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const controls = await canvas.findByRole("group", {
      name: "Locale verification controls",
    });
    const englishToolbar = await canvas.findByRole("region", {
      name: "dashboard summary",
    });
    await expect(
      within(controls).getByText("Current locale: en"),
    ).toBeVisible();
    await expect(
      within(englishToolbar).queryByRole("button", {
        name: "Return to current tick",
      }),
    ).toBeNull();
    await expect(
      within(englishToolbar).getByRole("slider", {
        name: "Timeline tick",
      }),
    ).toBeVisible();
    await expect(await canvas.findByText("5/5")).toBeVisible();

    await userEvent.click(
      within(controls).getByRole("button", {
        name: "Switch to zh-CN",
      }),
    );

    const mandarinToolbar = await canvas.findByRole("region", {
      name: "仪表板概览",
    });
    await expect(
      within(controls).getByText("Current locale: zh-CN"),
    ).toBeVisible();
    await expect(
      within(mandarinToolbar).queryByRole("button", {
        name: "返回当前刻度",
      }),
    ).toBeNull();
    await expect(
      within(mandarinToolbar).getByRole("slider", {
        name: "时间线刻度",
      }),
    ).toBeVisible();
    await expect(await canvas.findByText("5/5")).toBeVisible();
  },
};
