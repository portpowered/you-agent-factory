import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";

import { Button } from "./button";
import { ButtonLink } from "./button-link";
import { IconButtonShell } from "./icon-button-shell";

function RefreshIcon() {
  return (
    <svg aria-hidden="true" fill="none" viewBox="0 0 16 16">
      <path
        d="M13 8a5 5 0 1 1-1.46-3.54M13 3.5V7h-3.5"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}

function ExportIcon() {
  return (
    <svg aria-hidden="true" fill="none" viewBox="0 0 16 16">
      <path
        d="M8 2v8m0 0 3-3m-3 3L5 7M3 12.5h10"
        stroke="currentColor"
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}

function SemanticButtonVariantsShowcase() {
  return (
    <div className="grid gap-6">
      <section aria-labelledby="semantic-tones-heading" className="grid gap-3">
        <h2
          className="m-0 text-lg font-semibold text-on-surface"
          id="semantic-tones-heading"
        >
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
        <h2
          className="m-0 text-lg font-semibold text-on-surface"
          id="link-like-heading"
        >
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
        <h2
          className="m-0 text-lg font-semibold text-on-surface"
          id="disabled-heading"
        >
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
    docs: {
      description: {
        component:
          "Package-owned button primitives for semantic actions, link-like navigation, loading states, and icon-only toolbar controls. See docs/button.md for variant guidance and host-app responsibilities.",
      },
    },
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

function LoadingAndIconOnlyShowcase() {
  return (
    <div className="grid gap-6">
      <section aria-labelledby="loading-heading" className="grid gap-3">
        <h2
          className="m-0 text-lg font-semibold text-on-surface"
          id="loading-heading"
        >
          Loading states
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <Button loading type="button">
            Syncing graph
          </Button>
          <Button loading tone="destructive" type="button">
            Deleting factory
          </Button>
        </div>
      </section>

      <section aria-labelledby="icon-only-heading" className="grid gap-3">
        <h2
          className="m-0 text-lg font-semibold text-on-surface"
          id="icon-only-heading"
        >
          Icon-only actions
        </h2>
        <div className="flex flex-wrap items-center gap-3">
          <Button
            aria-label="Refresh jobs"
            size="icon"
            tone="outline"
            type="button"
          >
            <RefreshIcon />
          </Button>
          <IconButtonShell aria-label="Export dashboard">
            <ExportIcon />
          </IconButtonShell>
          <IconButtonShell aria-label="Remove item" tone="dangerGhost">
            <span aria-hidden="true">x</span>
          </IconButtonShell>
          <IconButtonShell aria-label="Export dashboard" loading>
            <ExportIcon />
          </IconButtonShell>
        </div>
      </section>

      <section aria-labelledby="focus-heading" className="grid gap-3">
        <h2
          className="m-0 text-lg font-semibold text-on-surface"
          id="focus-heading"
        >
          Focus-visible treatment
        </h2>
        <p className="m-0 text-sm text-on-surface-variant">
          Tab through the controls below to verify the shared focus ring remains
          visible for text, link-like, loading, and icon-only buttons.
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="button">Focusable text</Button>
          <ButtonLink href="/docs/getting-started" tone="secondary">
            Focusable link
          </ButtonLink>
          <Button loading type="button">
            Focusable loading
          </Button>
          <IconButtonShell aria-label="Focusable icon action">
            <RefreshIcon />
          </IconButtonShell>
        </div>
      </section>
    </div>
  );
}

export const LoadingAndIconOnly: Story = {
  render: () => <LoadingAndIconOnlyShowcase />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByText("Loading states")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Syncing graph" }),
    ).toHaveAttribute("aria-busy", "true");
    await expect(
      canvas.getByRole("button", { name: "Syncing graph" }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("button", { name: "Refresh jobs" }),
    ).toBeVisible();
    await expect(
      canvas.getAllByRole("button", { name: "Export dashboard" }),
    ).toHaveLength(2);
    await expect(
      canvas.getByRole("button", { name: "Remove item" }),
    ).toBeVisible();
    await expect(canvas.getByText("Focus-visible treatment")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Focusable text" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("link", { name: "Focusable link" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Focusable icon action" }),
    ).toBeVisible();

    const loadingButtons = canvas
      .getAllByRole("button")
      .filter((button) => button.getAttribute("aria-busy") === "true");
    await expect(loadingButtons.length).toBeGreaterThanOrEqual(2);
    for (const button of loadingButtons) {
      await expect(button.querySelector("svg.animate-spin")).toBeTruthy();
    }
  },
};
