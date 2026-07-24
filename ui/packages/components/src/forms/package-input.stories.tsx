import type { Meta, StoryObj } from "@storybook/react-vite";

import {
  ControlledInputStoryExample,
  PACKAGE_INPUT_STORY_LABEL,
  PackageFormStoryField,
  UncontrolledInputStoryExample,
  withMobileWidth,
} from "./package-form-story-support";
import { PackageInput } from "./package-input";

const meta = {
  title: "Forms/PackageInput",
  component: PackageInput,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof PackageInput>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Controlled: Story = {
  render: () => <ControlledInputStoryExample />,
};

export const Uncontrolled: Story = {
  render: () => <UncontrolledInputStoryExample />,
};

export const Disabled: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_INPUT_STORY_LABEL}>
      {(controlProps) => (
        <PackageInput {...controlProps} defaultValue="Locked" disabled />
      )}
    </PackageFormStoryField>
  ),
};

export const Invalid: Story = {
  render: () => (
    <PackageFormStoryField invalid label={PACKAGE_INPUT_STORY_LABEL}>
      {(controlProps) => (
        <PackageInput {...controlProps} defaultValue="Invalid value" />
      )}
    </PackageFormStoryField>
  ),
};

export const ErrorState: Story = {
  render: () => (
    <PackageFormStoryField
      errorText="Factory name is required."
      label={PACKAGE_INPUT_STORY_LABEL}
    >
      {(controlProps) => <PackageInput {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const Focus: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_INPUT_STORY_LABEL}>
      {(controlProps) => (
        <PackageInput
          {...controlProps}
          autoFocus
          defaultValue="Focused value"
        />
      )}
    </PackageFormStoryField>
  ),
};

export const HelperText: Story = {
  render: () => (
    <PackageFormStoryField
      helperText="Use the factory name shown in the dashboard header."
      label={PACKAGE_INPUT_STORY_LABEL}
    >
      {(controlProps) => (
        <PackageInput {...controlProps} placeholder="Factory name" />
      )}
    </PackageFormStoryField>
  ),
};

export const MobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => (
    <PackageFormStoryField
      helperText="Rendered at a 320px host width."
      label={PACKAGE_INPUT_STORY_LABEL}
    >
      {(controlProps) => (
        <PackageInput {...controlProps} placeholder="Factory name" />
      )}
    </PackageFormStoryField>
  ),
};
