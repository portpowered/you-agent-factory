import { expect, within } from "storybook/test";

import "../../../styles.css";
import { expectNoPageHorizontalOverflow } from "../../../stories/dashboardStorySupport";
import {
  buildEditableConfigurationDocument,
  CurrentSelectionCardStory,
  CurrentSelectionEditableConfigurationStory,
  CurrentSelectionWorkContentsCardStory,
  editableConfigurationPromptTemplateContract,
  expectBentoHeaderDragSurface,
  expectEditableConfigurationStoryFlow,
  promptTemplateValidationResponse,
  semanticWorkflowDashboardSnapshot,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const CurrentSelection = {
  render: () => <CurrentSelectionCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Current selection",
    });

    await expect(within(card).getByText("Implement")).toBeVisible();
    expectBentoHeaderDragSurface(card, "Current selection");
  },
};

export const CurrentSelectionWorkContents = {
  render: () => <CurrentSelectionWorkContentsCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Current selection",
    });
    const workContents = await within(card).findByRole("region", {
      name: "Work contents",
    });

    await expect(
      within(workContents).getByText("Primary selected-work payload text"),
    ).toBeVisible();
    await expect(within(workContents).getByText(/"priority": 1/)).toBeVisible();
    await expect(
      within(workContents).getByText("Image: screenshot.png"),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Move Current selection" }),
    ).toBeVisible();
  },
};

export const CurrentSelectionEditableConfigurationDesktop = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/~default/factory",
          response: {
            body: buildEditableConfigurationDocument(),
          },
        },
        {
          method: "GET",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-contract",
          response: {
            body: editableConfigurationPromptTemplateContract,
          },
        },
        {
          method: "POST",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: promptTemplateValidationResponse(init),
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <CurrentSelectionEditableConfigurationStory width={960} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectEditableConfigurationStoryFlow(canvasElement);
  },
};

export const CurrentSelectionEditableConfigurationNarrow = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/factory-sessions/~default/factory",
          response: {
            body: buildEditableConfigurationDocument(),
          },
        },
        {
          method: "GET",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-contract",
          response: {
            body: editableConfigurationPromptTemplateContract,
          },
        },
        {
          method: "POST",
          path: "/factory-sessions/~default/factory/workstations/Review/prompt-template-validation",
          response: (_input: RequestInfo | URL, init?: RequestInit) => ({
            body: promptTemplateValidationResponse(init),
          }),
        },
      ],
      snapshot: semanticWorkflowDashboardSnapshot,
    },
  },
  render: () => <CurrentSelectionEditableConfigurationStory width={360} />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expectEditableConfigurationStoryFlow(canvasElement);
    expectNoPageHorizontalOverflow(canvasElement);
  },
};
