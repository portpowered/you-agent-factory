import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import type { useTraceDrilldown } from "../../trace-drilldown/hooks/useTraceDrilldown";
import { TraceDrilldownWidget } from "../../trace-drilldown/public";
import { DASHBOARD_WIDGET_IDS } from "../hooks/dashboardLayoutSchema";
import {
  expectBentoHeaderDragSurface,
  layoutFor,
  renderCardFrame,
  renderTraceStateCard,
  storyTrace,
  TraceDrilldownInteractiveStory,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const TraceDrilldown = {
  render: () =>
    renderCardFrame({
      children: (
        <TraceDrilldownWidget
          state={
            {
              status: "ready",
              trace: storyTrace,
            } satisfies ReturnType<typeof useTraceDrilldown>["traceGridState"]
          }
          widgetId="trace::story"
        />
      ),
      layout: layoutFor(DASHBOARD_WIDGET_IDS.trace, {
        h: 8,
        id: "trace::story",
        w: 8,
      }),
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(within(card).getByText("trace-active-story")).toBeVisible();
    expectBentoHeaderDragSurface(card, "Trace drill-down");
  },
};

export const TraceDrilldownInteractive = {
  render: () => <TraceDrilldownInteractiveStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await userEvent.click(within(card).getByRole("button", { name: "Expand" }));
    await userEvent.click(
      within(card).getAllByRole("button", { name: /Active Story/ })[0],
    );

    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "Selected trace work item: work-active-story",
    );
  },
};

export const TraceDrilldownLoading = {
  render: () =>
    renderTraceStateCard({
      status: "loading",
      workID: "work-loading-story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(within(card).getByText("Loading trace")).toBeVisible();
    await expect(
      within(card).getByText(
        "Reconstructing dispatch history for work-loading-story.",
      ),
    ).toBeVisible();
  },
};

export const TraceDrilldownEmpty = {
  render: () =>
    renderTraceStateCard({
      status: "empty",
      workID: "work-empty-story",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(
      within(card).getByText("Trace history unavailable"),
    ).toBeVisible();
    await expect(
      within(card).getByText(
        "No retained dispatch history is currently available for this work item.",
      ),
    ).toBeVisible();
  },
};

export const TraceDrilldownError = {
  render: () =>
    renderTraceStateCard({
      message: "Trace history request failed.",
      status: "error",
    }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Trace drill-down",
    });

    await expect(within(card).getByText("Trace lookup failed")).toBeVisible();
    await expect(
      within(card).getByText("Trace history request failed."),
    ).toBeVisible();
  },
};
