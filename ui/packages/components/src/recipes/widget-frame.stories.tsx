import { expect, within } from "storybook/test";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

import {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetFrame,
  WidgetFrameDisclosure,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
  WidgetErrorState,
  WidgetLoadingState,
  WidgetSubtitle,
  WidgetSuccessState,
} from "./index";

const meta = {
  title: "Recipes/WidgetFrame",
  component: WidgetFrame,
  parameters: {
    layout: "padded",
  },
  args: {
    title: "Example widget",
    children: null,
  },
} satisfies Meta<typeof WidgetFrame>;

export default meta;

type Story = StoryObj<typeof meta>;

export const SuccessContent: Story = {
  render: () => (
    <div className="w-full max-w-xl">
      <WidgetFrame title="Example widget">
        <WidgetSubtitle>42 items</WidgetSubtitle>
        <WidgetDetailCopy>Host-provided detail copy.</WidgetDetailCopy>
      </WidgetFrame>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("article", { name: "Example widget" }),
    ).toBeVisible();
    await expect(canvas.getByText("42 items")).toBeVisible();
  },
};

export const EmptyState: Story = {
  render: () => (
    <div className="w-full max-w-xl">
      <WidgetFrame title="Example widget">
        <WidgetEmptyState>
          <WidgetEmptyStateTitle>No data yet</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            Provide content from the host application.
          </WidgetEmptyStateText>
        </WidgetEmptyState>
      </WidgetFrame>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("heading", { name: "No data yet" }),
    ).toBeVisible();
    await expect(
      canvas.getByText("Provide content from the host application."),
    ).toBeVisible();
  },
};

export const LoadingState: Story = {
  render: () => (
    <div className="w-full max-w-xl">
      <WidgetFrame title="Example widget">
        <WidgetLoadingState>
          <WidgetEmptyStateTitle>Loading content</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>Host-provided loading message.</WidgetEmptyStateText>
        </WidgetLoadingState>
      </WidgetFrame>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByRole("status")).toHaveAttribute("aria-busy", "true");
    await expect(
      canvas.getByRole("heading", { name: "Loading content" }),
    ).toBeVisible();
  },
};

export const ErrorState: Story = {
  render: () => (
    <div className="w-full max-w-xl">
      <WidgetFrame title="Example widget">
        <WidgetErrorState>
          <WidgetEmptyStateTitle>Request failed</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>Host-provided error message.</WidgetEmptyStateText>
        </WidgetErrorState>
      </WidgetFrame>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByRole("alert")).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Request failed" }),
    ).toBeVisible();
  },
};

export const SuccessState: Story = {
  render: () => (
    <div className="w-full max-w-xl">
      <WidgetFrame title="Example widget">
        <WidgetSuccessState>
          <WidgetEmptyStateTitle>Action completed</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>Host-provided success message.</WidgetEmptyStateText>
        </WidgetSuccessState>
      </WidgetFrame>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByRole("status")).toBeVisible();
    await expect(
      canvas.getByRole("heading", { name: "Action completed" }),
    ).toBeVisible();
  },
};

function CollapsibleSectionExample() {
  const [expanded, setExpanded] = useState(false);
  const panelID = "widget-frame-disclosure-panel";

  return (
    <div className="w-full max-w-xl">
      <WidgetFrame title="Example widget">
        <WidgetFrameDisclosure>
          <div className="flex items-center justify-between gap-3">
            <WidgetEmptyStateTitle as="h3" className="m-0">
              Details section
            </WidgetEmptyStateTitle>
            <WidgetFrameDisclosureTrigger
              controlsID={panelID}
              expanded={expanded}
              onExpandedChange={setExpanded}
            >
              {expanded ? "Collapse details" : "Expand details"}
            </WidgetFrameDisclosureTrigger>
          </div>
          <WidgetFrameDisclosurePanel expanded={expanded} id={panelID}>
            <WidgetDetailCopy>
              Host-provided disclosure content stays visible when expanded.
            </WidgetDetailCopy>
          </WidgetFrameDisclosurePanel>
        </WidgetFrameDisclosure>
      </WidgetFrame>
    </div>
  );
}

export const CollapsedDisclosure: Story = {
  render: () => <CollapsibleSectionExample />,
  play: async ({ canvasElement, userEvent }) => {
    const canvas = within(canvasElement);
    const expandButton = canvas.getByRole("button", { name: "Expand details" });

    await expect(expandButton).toHaveAttribute("aria-expanded", "false");
    await expect(
      canvas.queryByText(
        "Host-provided disclosure content stays visible when expanded.",
      ),
    ).toBeNull();

    await userEvent.click(expandButton);

    await expect(expandButton).toHaveAttribute("aria-expanded", "true");
    await expect(
      canvas.getByText(
        "Host-provided disclosure content stays visible when expanded.",
      ),
    ).toBeVisible();

    await userEvent.click(
      canvas.getByRole("button", { name: "Collapse details" }),
    );

    await expect(
      canvas.getByRole("button", { name: "Expand details" }),
    ).toHaveAttribute("aria-expanded", "false");
  },
};

export const ExpandedDisclosure: Story = {
  render: () => {
    const panelID = "widget-frame-disclosure-panel-expanded";

    return (
      <div className="w-full max-w-xl">
        <WidgetFrame title="Example widget">
          <WidgetFrameDisclosure>
            <div className="flex items-center justify-between gap-3">
              <WidgetEmptyStateTitle as="h3" className="m-0">
                Details section
              </WidgetEmptyStateTitle>
              <WidgetFrameDisclosureTrigger
                controlsID={panelID}
                expanded
              >
                Collapse details
              </WidgetFrameDisclosureTrigger>
            </div>
            <WidgetFrameDisclosurePanel expanded id={panelID}>
              <WidgetDetailCopy>
                Host-provided disclosure content stays visible when expanded.
              </WidgetDetailCopy>
            </WidgetFrameDisclosurePanel>
          </WidgetFrameDisclosure>
        </WidgetFrame>
      </div>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(
      canvas.getByRole("button", { name: "Collapse details" }),
    ).toHaveAttribute("aria-expanded", "true");
    await expect(
      canvas.getByText(
        "Host-provided disclosure content stays visible when expanded.",
      ),
    ).toBeVisible();
  },
};
