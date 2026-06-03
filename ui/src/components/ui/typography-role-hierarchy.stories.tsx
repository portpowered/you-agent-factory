import { expect, within } from "storybook/test";

import { TypographyRoleHierarchyShowcase } from "./typography-role-hierarchy";

export default {
  title: "Agent Factory/UI/Typography Role Hierarchy",
  component: TypographyRoleHierarchyShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <TypographyRoleHierarchyShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", { name: "Typography hierarchy (US-006)" }),
    ).toBeVisible();
    await expect(
      canvas.getByLabelText("Material typography and text color roles"),
    ).toBeVisible();
    await expect(canvas.getByText("on-primary-container")).toBeVisible();
    await expect(canvas.getByText("code / medium · code")).toBeVisible();
  },
};
