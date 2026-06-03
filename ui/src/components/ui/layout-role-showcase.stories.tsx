import { expect, within } from "storybook/test";

import { LayoutRoleShowcase } from "./layout-role-showcase";

export default {
  title: "Agent Factory/UI/Layout Role Primitives",
  component: LayoutRoleShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <LayoutRoleShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", { name: "Layout spacing (US-007)" }),
    ).toBeVisible();
    await expect(
      canvas.getByLabelText("Material layout spacing primitives"),
    ).toBeVisible();
    await expect(canvas.getByText("Page header layout")).toBeVisible();
    await expect(canvas.getByText("Stacked card layout")).toBeVisible();
    await expect(canvas.getByLabelText("Field label")).toBeVisible();
  },
};
