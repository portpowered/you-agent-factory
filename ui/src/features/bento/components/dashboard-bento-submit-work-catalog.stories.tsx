import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import { SubmitWorkWidget } from "../../submit-work/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";
import {
  expectBentoHeaderDragSurface,
  layoutFor,
  renderCardFrame,
  renderSubmitWorkStatusCard,
  SubmitWorkInteractiveStory,
  semanticWorkflowDashboardSnapshot,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const SubmitWork = {
  render: () =>
    renderCardFrame({
      children: (
        <SubmitWorkWidget
          submitWorkTypes={
            semanticWorkflowDashboardSnapshot.topology.submit_work_types
          }
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.submitWork, {
        h: 6,
        id: "submit-work::story",
        w: 5,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByRole("combobox", { name: "Work type" }),
    ).toBeVisible();
    expectBentoHeaderDragSurface(card, "Submit work");
  },
};

export const SubmitWorkInteractive = {
  render: () => <SubmitWorkInteractiveStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByRole("button", {
        name: "Remove Submit work widget from dashboard",
      }),
    ).toBeVisible();
    await userEvent.selectOptions(
      within(card).getByRole("combobox", { name: "Work type" }),
      "story",
    );
    await userEvent.type(
      within(card).getByRole("textbox", { name: "Request name" }),
      "Interactive coverage",
    );
    await userEvent.type(
      within(card).getByRole("textbox", { name: "Text item 1" }),
      "Verify the bento card interaction path.",
    );
    await userEvent.click(
      within(card).getByRole("button", { name: "Submit work" }),
    );

    await expect(await within(card).findByRole("status")).toHaveTextContent(
      "Submitted Interactive coverage as story.",
    );
  },
};

export const SubmitWorkEmpty = {
  render: () =>
    renderSubmitWorkStatusCard({
      status: "empty",
      storyID: "submit-work-empty::story",
      submitWorkTypeNames: [],
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByText(
        "No work types are available to submit right now.",
      ),
    ).toBeVisible();
    await expect(
      within(card).getByRole("button", { name: "Submit work" }),
    ).toBeDisabled();
  },
};

export const SubmitWorkSubmitting = {
  render: () =>
    renderSubmitWorkStatusCard({
      isSubmitting: true,
      status: "submitting",
      storyID: "submit-work-submitting::story",
      submitWorkTypeNames: ["story"],
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(
      within(card).getByRole("button", { name: "Submitting..." }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(
      within(card).getByText("Submitting work to the selected factory."),
    ).toBeVisible();
  },
};

export const SubmitWorkError = {
  render: () =>
    renderSubmitWorkStatusCard({
      status: "error",
      storyID: "submit-work-error::story",
      submitWorkTypeNames: ["story"],
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", { name: "Submit work" });

    await expect(await within(card).findByRole("alert")).toHaveTextContent(
      "Submission failed because the factory rejected the request.",
    );
    await expect(
      within(card).getByText("At least one text or file item is required."),
    ).toBeVisible();
  },
};
