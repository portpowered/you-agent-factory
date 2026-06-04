import { expect, within } from "storybook/test";

import { ColorRoleNeutralSurfacesShowcase } from "./color-role-neutral-surfaces";

export default {
  title: "Agent Factory/UI/Color Role Neutral Surfaces",
  component: ColorRoleNeutralSurfacesShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <ColorRoleNeutralSurfacesShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", {
        name: "Neutral surface layering (US-005)",
      }),
    ).toBeVisible();
    await expect(
      canvas.getByLabelText("Material neutral surface role layers"),
    ).toBeVisible();
    await expect(canvas.getByText("surface-container-low")).toBeVisible();
    await expect(canvas.getByText("surface-container-high")).toBeVisible();
  },
};
