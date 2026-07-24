import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  AlertPanel,
  type AlertPanelSemanticVariant,
  AlertPanelText,
  AlertPanelTitle,
} from "./alert-panel";

const semanticVariants = [
  "neutral",
  "info",
  "success",
  "warning",
  "danger",
  "error",
  "loading",
  "empty",
] as const satisfies readonly AlertPanelSemanticVariant[];

const meta = {
  title: "Feedback/AlertPanel",
  component: AlertPanel,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof AlertPanel>;

export default meta;

type Story = StoryObj<typeof meta>;

export const SemanticVariants: Story = {
  render: () => (
    <div className="grid max-w-xl gap-3">
      {semanticVariants.map((semantic) => (
        <AlertPanel key={semantic} semantic={semantic}>
          {semantic === "loading" ? null : (
            <>
              <AlertPanelTitle>
                {semantic.charAt(0).toUpperCase() + semantic.slice(1)} feedback
              </AlertPanelTitle>
              <AlertPanelText>
                Package-owned semantic feedback for the {semantic} state.
              </AlertPanelText>
            </>
          )}
        </AlertPanel>
      ))}
    </div>
  ),
};

export const Neutral: Story = {
  args: {
    semantic: "neutral",
    children: (
      <>
        <AlertPanelTitle>Neutral feedback</AlertPanelTitle>
        <AlertPanelText>Informational copy without urgency.</AlertPanelText>
      </>
    ),
  },
};

export const Info: Story = {
  args: {
    semantic: "info",
    children: (
      <>
        <AlertPanelTitle>Info feedback</AlertPanelTitle>
        <AlertPanelText>
          Helpful context for the current surface.
        </AlertPanelText>
      </>
    ),
  },
};

export const Success: Story = {
  args: {
    semantic: "success",
    children: (
      <>
        <AlertPanelTitle>Success feedback</AlertPanelTitle>
        <AlertPanelText>
          The requested action completed successfully.
        </AlertPanelText>
      </>
    ),
  },
};

export const Warning: Story = {
  args: {
    semantic: "warning",
    children: (
      <>
        <AlertPanelTitle>Warning feedback</AlertPanelTitle>
        <AlertPanelText>
          Review the highlighted details before continuing.
        </AlertPanelText>
      </>
    ),
  },
};

export const Danger: Story = {
  args: {
    semantic: "danger",
    children: (
      <>
        <AlertPanelTitle>Danger feedback</AlertPanelTitle>
        <AlertPanelText>
          This action may have destructive consequences.
        </AlertPanelText>
      </>
    ),
  },
};

export const ErrorState: Story = {
  args: {
    semantic: "error",
    children: (
      <>
        <AlertPanelTitle>Request failed</AlertPanelTitle>
        <AlertPanelText>
          The server rejected the latest submission.
        </AlertPanelText>
      </>
    ),
  },
};

export const Loading: Story = {
  args: {
    semantic: "loading",
  },
};

export const Empty: Story = {
  args: {
    semantic: "empty",
    children: (
      <>
        <AlertPanelTitle>No results yet</AlertPanelTitle>
        <AlertPanelText>Submit work to populate this surface.</AlertPanelText>
      </>
    ),
  },
};
