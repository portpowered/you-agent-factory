import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./package-select";
import {
  ControlledOpenSelectStoryExample,
  ControlledSelectStoryExample,
  EmptyOptionsSelectStoryExample,
  ErrorStateSelectStoryExample,
  KeyboardSelectStoryExample,
  LoadingOptionsSelectStoryExample,
  LongLabelSelectStoryExample,
  PACKAGE_SELECT_STORY_LABEL,
  PackageSelectStoryField,
  withMobileWidth,
} from "./package-select-story-support";

const meta = {
  title: "Forms/PackageSelect",
  component: Select,
  parameters: {
    layout: "centered",
    docs: {
      description: {
        component:
          "Package-owned Radix select primitives with keyboard navigation, controlled value/open usage, and disabled option handling.",
      },
    },
  },
  tags: ["test"],
} satisfies Meta<typeof Select>;

export default meta;

type Story = StoryObj<typeof meta>;

export const ControlledValue: Story = {
  render: () => <ControlledSelectStoryExample />,
};

export const ControlledOpen: Story = {
  render: () => <ControlledOpenSelectStoryExample />,
};

export const KeyboardInteraction: Story = {
  render: () => <KeyboardSelectStoryExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = canvas.getByRole("combobox", {
      name: PACKAGE_SELECT_STORY_LABEL,
    });

    trigger.focus();
    await userEvent.keyboard("{ArrowDown}");

    const listbox = await canvas.findByRole("listbox");
    const storyOption = within(listbox).getByRole("option", { name: "Story" });
    await expect(storyOption).toBeVisible();

    await userEvent.keyboard("{Enter}");
    await expect(trigger).toHaveTextContent("Story");
    await expect(canvas.queryByRole("listbox")).toBeNull();
    await expect(trigger).toHaveFocus();
  },
};

export const DisabledField: Story = {
  render: () => (
    <PackageSelectStoryField label={PACKAGE_SELECT_STORY_LABEL}>
      {(controlProps) => (
        <Select disabled value="story">
          <SelectTrigger
            aria-label={PACKAGE_SELECT_STORY_LABEL}
            {...controlProps}
          >
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="story">Story</SelectItem>
            <SelectItem value="bug">Bug</SelectItem>
          </SelectContent>
        </Select>
      )}
    </PackageSelectStoryField>
  ),
};

export const DisabledOption: Story = {
  render: () => (
    <PackageSelectStoryField label={PACKAGE_SELECT_STORY_LABEL}>
      {(controlProps) => (
        <Select value="story">
          <SelectTrigger
            aria-label={PACKAGE_SELECT_STORY_LABEL}
            {...controlProps}
          >
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="story">Story</SelectItem>
            <SelectItem disabled value="task">
              Task
            </SelectItem>
            <SelectItem value="bug">Bug</SelectItem>
          </SelectContent>
        </Select>
      )}
    </PackageSelectStoryField>
  ),
};

export const Focus: Story = {
  render: () => (
    <PackageSelectStoryField label={PACKAGE_SELECT_STORY_LABEL}>
      {(controlProps) => (
        <Select defaultValue="story">
          <SelectTrigger
            aria-label={PACKAGE_SELECT_STORY_LABEL}
            autoFocus
            {...controlProps}
          >
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="story">Story</SelectItem>
            <SelectItem value="bug">Bug</SelectItem>
          </SelectContent>
        </Select>
      )}
    </PackageSelectStoryField>
  ),
};

export const MobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => <ControlledSelectStoryExample />,
};

export const EmptyOptions: Story = {
  render: () => <EmptyOptionsSelectStoryExample />,
};

export const LoadingOptions: Story = {
  render: () => <LoadingOptionsSelectStoryExample />,
};

export const ErrorState: Story = {
  render: () => <ErrorStateSelectStoryExample />,
};

export const LongLabel: Story = {
  render: () => <LongLabelSelectStoryExample />,
};

export const LongLabelMobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => <LongLabelSelectStoryExample />,
};
