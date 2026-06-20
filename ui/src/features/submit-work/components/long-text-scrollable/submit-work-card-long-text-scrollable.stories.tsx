import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import meta from "../submit-work-card.stories";
import { SubmitWorkCardLongTextScrollableVerification } from "./submit-work-card-long-text-scrollable-verification";

export default {
  ...meta,
  title: "Agent Factory/Dashboard/Submit Work Card",
} satisfies Meta;

type Story = StoryObj;

export const LongTextScrollableVerification: Story = {
  tags: ["test"],
  render: () => <SubmitWorkCardLongTextScrollableVerification />,
  play: async ({ canvasElement }) => {
    const card = await within(canvasElement).findByRole("article", {
      name: "Submit work",
    });
    const scope = within(card);
    const submissionTextarea = scope.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    await expect(submissionTextarea.className).toContain("max-h-52");
    await expect(submissionTextarea.className).toContain("overflow-y-auto");
    await expect(submissionTextarea.className).toContain("af-styled-scrollbar");
    await expect(submissionTextarea.scrollHeight).toBeGreaterThan(
      submissionTextarea.clientHeight,
    );
    await expect(
      scope.getByRole("button", { name: "Submit work" }),
    ).toBeVisible();
  },
};
