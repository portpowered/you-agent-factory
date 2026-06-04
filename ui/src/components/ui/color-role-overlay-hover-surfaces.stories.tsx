import "../../styles.css";

import { expect, within } from "storybook/test";

import { ColorRoleOverlayHoverSurfacesShowcase } from "./color-role-overlay-hover-surfaces";

export default {
  title: "Agent Factory/UI/Color Role Overlay Hover Surfaces",
  component: ColorRoleOverlayHoverSurfacesShowcase,
  tags: ["test"],
};

export const OverlayHoverPaletteVerification = {
  render: () => <ColorRoleOverlayHoverSurfacesShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", {
        name: "Overlay hover migration (surface-container roles)",
      }),
    ).toBeVisible();
    await expect(canvas.getByTestId("hover-ghost-button")).toBeVisible();
    await expect(canvas.getByTestId("hover-outline-button")).toBeVisible();
    await expect(canvas.getByTestId("hover-secondary-button")).toBeVisible();
    await expect(canvas.getByTestId("hover-table-row")).toBeVisible();
    await expect(canvas.getByTestId("selected-table-row")).toBeVisible();
    await expect(canvas.getByTestId("hover-list-row")).toBeVisible();
    await expect(canvas.getByTestId("hover-panel-section")).toBeVisible();
    await expect(canvas.getByTestId("hover-panel-compact")).toBeVisible();
  },
};
