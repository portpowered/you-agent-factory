import { expect, within } from "storybook/test";
import { applyDocumentColorPalette } from "../../theme/app-color-palette";
import { COLOR_PALETTE_IDS } from "../../theme/color-palette";
import { ColorRoleAccentContrastShowcase } from "./color-role-accent-contrast";

export default {
  title: "Agent Factory/UI/Color Role Accent Contrast",
  component: ColorRoleAccentContrastShowcase,
  tags: ["test"],
};

export const Default = {
  render: () => <ColorRoleAccentContrastShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", { name: "Accent role contrast (US-003)" }),
    ).toBeVisible();
    await expect(canvas.getByText("Primary")).toBeVisible();
    await expect(canvas.getByText("Secondary")).toBeVisible();
    await expect(canvas.getByText("Tertiary")).toBeVisible();
    await expect(
      canvas.getByText("Prior secondary hue (info foundation)"),
    ).toBeVisible();
    await expect(
      canvas.getByText("Prior tertiary hue (worker foundation)"),
    ).toBeVisible();
    await expect(
      canvas.getByLabelText("Material accent role swatches"),
    ).toBeVisible();
    await expect(
      canvas.getByLabelText("Legacy vibrant accent references"),
    ).toBeVisible();
  },
};

export const PaletteSwitching = {
  render: () => <ColorRoleAccentContrastShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    for (const paletteId of COLOR_PALETTE_IDS) {
      applyDocumentColorPalette(paletteId);

      await expect(document.documentElement.dataset.colorPalette).toBe(
        paletteId,
      );
      await expect(canvas.getByText("Role fill")).toBeVisible();
      await expect(
        canvas.getByText("Container + on-container ink"),
      ).toBeVisible();
    }

    applyDocumentColorPalette("factory-dark");
  },
};
