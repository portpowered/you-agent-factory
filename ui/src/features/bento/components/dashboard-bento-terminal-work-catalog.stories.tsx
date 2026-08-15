import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import { TerminalWorkWidget } from "../../terminal-work/components/terminal-work-widget";
import { getTerminalWorkMessages } from "../../terminal-work/messages/terminal-work";
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
          selectedItem={{
            status: "failed",
            traceWorkID: "work-failed-story",
          }}
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
    const messages = getTerminalWorkMessages("en");
    const card = await canvas.findByRole("article", {
      name: messages.cardTitle,
    });
    const terminalScope = within(card);

    await expect(
      terminalScope.getByRole("button", {
        name: messages.selectWorkItemLabel("Failed Story"),
      }),
    ).toBeVisible();

    const completedToggle = (
      await terminalScope.findAllByRole("button", {
        name: messages.disclosureLabel(true),
      })
    )[0];
    await expect(completedToggle).toHaveAttribute("aria-expanded", "true");
    await userEvent.click(completedToggle);
    await expect(completedToggle).toHaveAttribute("aria-expanded", "false");
    expect(
      terminalScope.queryByRole("button", {
        name: messages.selectWorkItemLabel("Done Story"),
      }),
    ).toBeNull();
    await userEvent.click(completedToggle);
    await expect(completedToggle).toHaveAttribute("aria-expanded", "true");
    await expect(
      terminalScope.getByRole("button", {
        name: messages.selectWorkItemLabel("Done Story"),
      }),
    ).toBeVisible();
    expectBentoHeaderDragSurface(card, messages.cardTitle);
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
    const messages = getTerminalWorkMessages("en");
    const card = await canvas.findByRole("article", {
      name: messages.cardTitle,
    });

    await expect(
      within(card).getByText(messages.emptyState("completed")),
    ).toBeVisible();
    await expect(
      within(card).getByText(messages.emptyState("failed")),
    ).toBeVisible();
  },
};
