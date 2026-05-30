import { expect, userEvent, within } from "storybook/test";

import "../../../styles.css";
import {
  expectBentoHeaderDragSurface,
  InlineAddWidgetCardStory,
} from "./dashboard-bento-story-shared";

export default {
  title: "you-agent-factory/Dashboard/Bento Cards",
  tags: ["test"],
};

export const InlineAddWidget = {
  render: () => <InlineAddWidgetCardStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const page = within(canvasElement.ownerDocument.body);
    const card = await canvas.findByRole("article", { name: "Add widget" });
    const addWidgetButton = within(card).getByRole("button", {
      name: "Add widget",
    });

    await expect(addWidgetButton).toBeVisible();
    addWidgetButton.focus();
    await expect(addWidgetButton).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    const dialog = await page.findByRole("dialog", {
      name: "Add dashboard widget",
    });

    await expect(dialog).toBeVisible();
    await expect(
      within(dialog).getByRole("button", {
        name: "Browse widgets: Current selection",
      }),
    ).toBeDisabled();
    await userEvent.click(
      within(dialog).getByRole("button", {
        name: "Browse widgets: Work totals",
      }),
    );
    await expect(await canvas.findByRole("status")).toHaveTextContent(
      "Selected widget: work-totals",
    );
    expectBentoHeaderDragSurface(card, "Add widget");
  },
};
