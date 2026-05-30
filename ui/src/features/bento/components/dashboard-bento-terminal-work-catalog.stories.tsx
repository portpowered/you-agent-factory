import { expect, within } from "storybook/test";

import "../../../styles.css";
import { TerminalWorkWidget } from "../../terminal-work/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";
import {
  completedAttempt,
  expectBentoHeaderDragSurface,
  failedAttempt,
  layoutFor,
  renderCardFrame,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const TerminalWork = {
  render: () =>
    renderCardFrame({
      children: (
        <TerminalWorkWidget
          completedItems={[
            {
              attempts: [completedAttempt],
              label: "Done Story",
              traceWorkID: "work-done-story",
            },
          ]}
          failedItems={[
            {
              attempts: [failedAttempt],
              label: "Failed Story",
              traceWorkID: "work-failed-story",
            },
          ]}
          onSelectItem={() => undefined}
          selectedItem={{ label: "Failed Story", status: "failed" }}
          widgetId="terminal-work::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.terminalWork, {
        h: 5,
        id: "terminal-work::story",
        w: 5,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Completed and failed work",
    });

    await expect(
      within(card).getByRole("button", { name: "Failed Story" }),
    ).toBeVisible();
    expectBentoHeaderDragSurface(card, "Completed and failed work");
  },
};

export const TerminalWorkEmpty = {
  render: () =>
    renderCardFrame({
      children: (
        <TerminalWorkWidget
          completedItems={[]}
          failedItems={[]}
          onSelectItem={() => undefined}
          selectedItem={null}
          widgetId="terminal-work-empty::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.terminalWork, {
        h: 5,
        id: "terminal-work-empty::story",
        w: 5,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Completed and failed work",
    });

    await expect(
      within(card).getByText("No completed work recorded yet."),
    ).toBeVisible();
    await expect(
      within(card).getByText("No failed work recorded yet."),
    ).toBeVisible();
  },
};
