import { expect, within } from "storybook/test";

import { ThemeRoleMigrationOverview } from "./theme-role-migration-overview";

export default {
  title: "Agent Factory/UI/Theme Role Migration Overview",
  component: ThemeRoleMigrationOverview,
  tags: ["test"],
};

export const Default = {
  render: () => <ThemeRoleMigrationOverview />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("article", {
        name: "Material theme role migration overview",
      }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Theme role migration overview" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Accent hierarchy" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Neutral surfaces" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Typography hierarchy" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Layout primitives" }),
    ).toBeVisible();
  },
};
