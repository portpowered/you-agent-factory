import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormField } from "./package-form-field";
import {
  PACKAGE_FORM_FIELD_LONG_LABEL,
  PACKAGE_FORM_FIELD_LONG_MESSAGE,
  PACKAGE_FORM_FIELD_STORY_DESCRIPTION,
  PACKAGE_FORM_FIELD_STORY_ERROR,
  PACKAGE_FORM_FIELD_STORY_HELPER,
  PACKAGE_FORM_FIELD_STORY_SUCCESS,
  PACKAGE_FORM_FIELD_STORY_WARNING,
  PackageFormFieldGroupedControlStoryExample,
  PackageFormFieldStoryExample,
  withMobileWidth,
  withStoryWidth,
} from "./package-form-field-story-support";

const meta = {
  title: "Forms/FormField",
  component: FormField,
  decorators: [withStoryWidth],
  parameters: {
    layout: "centered",
    docs: {
      description: {
        component:
          "Package-owned form-field structure and messaging with host-supplied labels, descriptions, validation text, and aria relationships.",
      },
    },
  },
} satisfies Meta<typeof FormField>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      defaultValue="Sample value"
      description={PACKAGE_FORM_FIELD_STORY_DESCRIPTION}
    />
  ),
};

export const Required: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      description={PACKAGE_FORM_FIELD_STORY_DESCRIPTION}
      required
      requiredAffordance="*"
    />
  ),
};

export const Disabled: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      defaultValue="Locked value"
      description={PACKAGE_FORM_FIELD_STORY_DESCRIPTION}
      disabled
    />
  ),
};

export const HelperText: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      helperText={PACKAGE_FORM_FIELD_STORY_HELPER}
    />
  ),
};

export const Warning: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      warningText={PACKAGE_FORM_FIELD_STORY_WARNING}
    />
  ),
};

export const ErrorState: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      errorText={PACKAGE_FORM_FIELD_STORY_ERROR}
      invalid
      useAriaErrorMessage
    />
  ),
};

export const Success: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      successText={PACKAGE_FORM_FIELD_STORY_SUCCESS}
    />
  ),
};

export const LongMessage: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      description={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      errorText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      helperText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      invalid
      label={PACKAGE_FORM_FIELD_LONG_LABEL}
      successText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      useAriaErrorMessage
      warningText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
    />
  ),
};

export const GroupedControl: Story = {
  render: () => <PackageFormFieldGroupedControlStoryExample />,
};

export const Focus: Story = {
  render: () => (
    <PackageFormFieldStoryExample
      autoFocus
      defaultValue="Focused value"
      helperText={PACKAGE_FORM_FIELD_STORY_HELPER}
    />
  ),
};

export const MobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => (
    <PackageFormFieldStoryExample
      description={PACKAGE_FORM_FIELD_STORY_DESCRIPTION}
      errorText={PACKAGE_FORM_FIELD_STORY_ERROR}
      helperText={PACKAGE_FORM_FIELD_STORY_HELPER}
      invalid
      successText={PACKAGE_FORM_FIELD_STORY_SUCCESS}
      useAriaErrorMessage
      warningText={PACKAGE_FORM_FIELD_STORY_WARNING}
    />
  ),
};

export const LongMessageMobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => (
    <PackageFormFieldStoryExample
      description={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      errorText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      helperText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      invalid
      label={PACKAGE_FORM_FIELD_LONG_LABEL}
      successText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
      useAriaErrorMessage
      warningText={PACKAGE_FORM_FIELD_LONG_MESSAGE}
    />
  ),
};
