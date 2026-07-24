import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { expect, within } from "storybook/test";

import {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetErrorState,
  WidgetFrame,
  WidgetFrameDisclosure,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
  WidgetLoadingState,
  WidgetSubtitle,
  WidgetSuccessState,
} from "./index";
import {
  WIDGET_FRAME_RESPONSIVE_SHELL_CLASS,
  WIDGET_FRAME_STORY_SHELL_DATA_ATTR,
  widgetFrameHasNoHorizontalOverflow,
  widgetFrameStoryShellStyle,
} from "./widget-frame-layout";

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

function ResponsiveWidgetFrameRecipeExample() {
  const [expanded, setExpanded] = useState(false);
  const panelID = "widget-frame-responsive-disclosure-panel";

  return (
    <WidgetFrame
      className={WIDGET_FRAME_RESPONSIVE_SHELL_CLASS}
      headerAction={
        <button
          className="shrink-0 rounded-lg border border-outline px-3 py-2 text-body-medium"
          type="button"
        >
          Refresh
        </button>
      }
      title="Example widget with a longer heading label"
    >
      <WidgetSubtitle>42 host-provided items</WidgetSubtitle>
      <WidgetDetailCopy>
        Host-provided detail copy stays readable while the frame shrinks to
        compact dashboard columns.
      </WidgetDetailCopy>
      <WidgetErrorState>
        <WidgetEmptyStateTitle>Request failed</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Host-provided error message for the responsive layout recipe.
        </WidgetEmptyStateText>
      </WidgetErrorState>
      <WidgetFrameDisclosure>
        <div className="flex min-w-0 flex-wrap items-center justify-between gap-3">
          <WidgetEmptyStateTitle as="h3" className="m-0 min-w-0 flex-1">
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
      <WidgetEmptyState compact>
        <WidgetEmptyStateTitle>Custom slot content</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Children can supply arbitrary host content without a dashboard shape.
        </WidgetEmptyStateText>
      </WidgetEmptyState>
    </WidgetFrame>
  );
}

function renderResponsiveWidgetFrameStoryShell(
  maxWidth: string,
  children: ReactNode = <ResponsiveWidgetFrameRecipeExample />,
) {
  return (
    <div
      {...widgetFrameStoryShellStyle(maxWidth)}
      {...{ [WIDGET_FRAME_STORY_SHELL_DATA_ATTR]: "true" }}
    >
      {children}
    </div>
  );
}

async function expectResponsiveWidgetFrameRecipeContract(
  canvasElement: HTMLElement,
) {
  const canvas = within(canvasElement);
  const shell = canvasElement.querySelector<HTMLElement>(
    `[${WIDGET_FRAME_STORY_SHELL_DATA_ATTR}]`,
  );

  expect(shell).not.toBeNull();
  expect(widgetFrameHasNoHorizontalOverflow(shell as HTMLElement)).toBe(true);

  const frame = await canvas.findByRole("article", {
    name: "Example widget with a longer heading label",
  });
  const title = within(frame).getByRole("heading", {
    level: 3,
    name: "Example widget with a longer heading label",
  });
  const refreshButton = within(frame).getByRole("button", { name: "Refresh" });
  const expandButton = within(frame).getByRole("button", {
    name: "Expand details",
  });

  await expect(title).toBeVisible();
  await expect(refreshButton).toBeVisible();
  await expect(within(frame).getByText("42 host-provided items")).toBeVisible();
  await expect(within(frame).getByRole("alert")).toBeVisible();
  await expect(expandButton).toBeVisible();

  const titleBox = title.getBoundingClientRect();
  const refreshBox = refreshButton.getBoundingClientRect();

  expect(
    titleBox.bottom <= refreshBox.top + 1 ||
      titleBox.right <= refreshBox.left + 1,
  ).toBe(true);
  expect((frame.scrollWidth ?? 0) <= (shell?.clientWidth ?? 0) + 1).toBe(true);
  expect(
    expandButton.getBoundingClientRect().right <=
      (shell?.getBoundingClientRect().right ?? 0) + 1,
  ).toBe(true);
}

export const ResponsiveCompact: Story = {
  render: () => renderResponsiveWidgetFrameStoryShell("360px"),
  play: async ({ canvasElement }) => {
    await expectResponsiveWidgetFrameRecipeContract(canvasElement);
  },
};

export const ResponsiveMedium: Story = {
  render: () => renderResponsiveWidgetFrameStoryShell("768px"),
  play: async ({ canvasElement }) => {
    await expectResponsiveWidgetFrameRecipeContract(canvasElement);
  },
};

export const ResponsiveWide: Story = {
  render: () => renderResponsiveWidgetFrameStoryShell("1280px"),
  play: async ({ canvasElement }) => {
    await expectResponsiveWidgetFrameRecipeContract(canvasElement);
  },
};

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
          <WidgetEmptyStateText>
            Host-provided loading message.
          </WidgetEmptyStateText>
        </WidgetLoadingState>
      </WidgetFrame>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    await expect(canvas.getByRole("status")).toHaveAttribute(
      "aria-busy",
      "true",
    );
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
          <WidgetEmptyStateText>
            Host-provided error message.
          </WidgetEmptyStateText>
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
          <WidgetEmptyStateText>
            Host-provided success message.
          </WidgetEmptyStateText>
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
              <WidgetFrameDisclosureTrigger controlsID={panelID} expanded>
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
