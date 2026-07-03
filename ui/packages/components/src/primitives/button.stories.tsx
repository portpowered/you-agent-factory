import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import { Button } from "./button";
import { ButtonLink } from "./button-link";

function SemanticButtonVariantsShowcase() {
  return (
    <div className="grid gap-6">
      <section aria-labelledby="semantic-tones-heading" className="grid gap-3">
        <h2 className="m-0 text-lg font-semibold text-on-surface" id="semantic-tones-heading">
          Semantic button tones
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="button">Primary action</Button>
          <Button tone="secondary" type="button">
            Secondary action
          </Button>
          <Button tone="outline" type="button">
            Outline action
          </Button>
          <Button tone="ghost" type="button">
            Ghost action
          </Button>
          <Button tone="destructive" type="button">
            Delete factory
          </Button>
          <Button tone="warning" type="button">
            Review warning
          </Button>
        </div>
      </section>

      <section aria-labelledby="link-like-heading" className="grid gap-3">
        <h2 className="m-0 text-lg font-semibold text-on-surface" id="link-like-heading">
          Link-like actions
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <ButtonLink href="/docs/getting-started" tone="secondary">
            Open docs
          </ButtonLink>
          <ButtonLink href="/settings" tone="outline">
            Settings
          </ButtonLink>
        </div>
      </section>

      <section aria-labelledby="disabled-heading" className="grid gap-3">
        <h2 className="m-0 text-lg font-semibold text-on-surface" id="disabled-heading">
          Disabled states
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <Button disabled type="button">
            Disabled primary
          </Button>
          <Button disabled tone="destructive" type="button">
            Disabled destructive
          </Button>
          <Button disabled tone="outline" type="button">
            Disabled outline
          </Button>
        </div>
      </section>
    </div>
  );
}

const meta = {
  title: "Primitives/Button",
  component: Button,
  parameters: {
    layout: "centered",
  },
  tags: ["test"],
} satisfies Meta<typeof Button>;

export default meta;

type Story = StoryObj<typeof meta>;

export const SemanticVariants: Story = {
  render: () => <SemanticButtonVariantsShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("Semantic button tones")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Primary action" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Secondary action" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Outline action" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Ghost action" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Delete factory" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Review warning" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("link", { name: "Open docs" }),
    ).toHaveAttribute("href", "/docs/getting-started");
    await expect(
      canvas.getByRole("button", { name: "Disabled primary" }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("button", { name: "Disabled destructive" }),
    ).toBeDisabled();
  },
};
