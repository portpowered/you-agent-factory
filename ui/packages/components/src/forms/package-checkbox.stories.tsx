import type { Meta, StoryObj } from "@storybook/react-vite";
import { PackageCheckbox } from "./package-checkbox";
import {
  ControlledCheckboxStoryExample,
  PACKAGE_CHECKBOX_STORY_LABEL,
  PackageFormStoryField,
  UncontrolledCheckboxStoryExample,
  withMobileWidth,
} from "./package-form-story-support";

const meta = {
  title: "Forms/PackageCheckbox",
  component: PackageCheckbox,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof PackageCheckbox>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Controlled: Story = {
  render: () => <ControlledCheckboxStoryExample />,
};

export const Uncontrolled: Story = {
  render: () => <UncontrolledCheckboxStoryExample />,
};

export const Disabled: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_CHECKBOX_STORY_LABEL}>
      {(controlProps) => (
        <PackageCheckbox {...controlProps} defaultChecked disabled />
      )}
    </PackageFormStoryField>
  ),
};

export const Invalid: Story = {
  render: () => (
    <PackageFormStoryField invalid label={PACKAGE_CHECKBOX_STORY_LABEL}>
      {(controlProps) => <PackageCheckbox {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const ErrorState: Story = {
  render: () => (
    <PackageFormStoryField
      errorText="Confirmation is required."
      label={PACKAGE_CHECKBOX_STORY_LABEL}
    >
      {(controlProps) => <PackageCheckbox {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const Focus: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_CHECKBOX_STORY_LABEL}>
      {(controlProps) => <PackageCheckbox {...controlProps} autoFocus />}
    </PackageFormStoryField>
  ),
};

export const HelperText: Story = {
  render: () => (
    <PackageFormStoryField
      helperText="Runs the cron trigger when the factory session starts."
      label={PACKAGE_CHECKBOX_STORY_LABEL}
    >
      {(controlProps) => <PackageCheckbox {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const MobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => (
    <PackageFormStoryField
      helperText="Rendered at a 320px host width."
      label={PACKAGE_CHECKBOX_STORY_LABEL}
    >
      {(controlProps) => <PackageCheckbox {...controlProps} />}
    </PackageFormStoryField>
  ),
};
