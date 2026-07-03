import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  ControlledTextareaStoryExample,
  PACKAGE_TEXTAREA_STORY_LABEL,
  PackageFormStoryField,
  UncontrolledTextareaStoryExample,
  withMobileWidth,
} from "./package-form-story-support";
import { PackageTextarea } from "./package-textarea";

const meta = {
  title: "Forms/PackageTextarea",
  component: PackageTextarea,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof PackageTextarea>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Controlled: Story = {
  render: () => <ControlledTextareaStoryExample />,
};

export const Uncontrolled: Story = {
  render: () => <UncontrolledTextareaStoryExample />,
};

export const Disabled: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_TEXTAREA_STORY_LABEL}>
      {(controlProps) => (
        <PackageTextarea
          {...controlProps}
          defaultValue="Locked notes"
          disabled
        />
      )}
    </PackageFormStoryField>
  ),
};

export const Invalid: Story = {
  render: () => (
    <PackageFormStoryField invalid label={PACKAGE_TEXTAREA_STORY_LABEL}>
      {(controlProps) => (
        <PackageTextarea {...controlProps} defaultValue="Invalid notes" />
      )}
    </PackageFormStoryField>
  ),
};

export const ErrorState: Story = {
  render: () => (
    <PackageFormStoryField
      errorText="Factory notes are required."
      label={PACKAGE_TEXTAREA_STORY_LABEL}
    >
      {(controlProps) => <PackageTextarea {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const Focus: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_TEXTAREA_STORY_LABEL}>
      {(controlProps) => (
        <PackageTextarea
          {...controlProps}
          autoFocus
          defaultValue="Focused notes"
        />
      )}
    </PackageFormStoryField>
  ),
};

export const HelperText: Story = {
  render: () => (
    <PackageFormStoryField
      helperText="Add context that operators will see in the run summary."
      label={PACKAGE_TEXTAREA_STORY_LABEL}
    >
      {(controlProps) => (
        <PackageTextarea {...controlProps} placeholder="Factory notes" />
      )}
    </PackageFormStoryField>
  ),
};

export const MobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => (
    <PackageFormStoryField
      helperText="Rendered at a 320px host width."
      label={PACKAGE_TEXTAREA_STORY_LABEL}
    >
      {(controlProps) => (
        <PackageTextarea {...controlProps} placeholder="Factory notes" />
      )}
    </PackageFormStoryField>
  ),
};
