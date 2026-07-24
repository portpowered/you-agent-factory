import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { PackageFileInput } from "./package-file-input";
import {
  PACKAGE_FILE_INPUT_STORY_LABEL,
  PackageFormStoryField,
  SelectedFileInputStoryExample,
  withMobileWidth,
} from "./package-form-story-support";

const meta = {
  title: "Forms/PackageFileInput",
  component: PackageFileInput,
  parameters: {
    layout: "centered",
  },
} satisfies Meta<typeof PackageFileInput>;

export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {
  render: () => (
    <PackageFormStoryField
      helperText="PNG or JPEG up to 2 MB."
      label={PACKAGE_FILE_INPUT_STORY_LABEL}
    >
      {(controlProps) => <PackageFileInput {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const Disabled: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_FILE_INPUT_STORY_LABEL}>
      {(controlProps) => <PackageFileInput {...controlProps} disabled />}
    </PackageFormStoryField>
  ),
};

export const Invalid: Story = {
  render: () => (
    <PackageFormStoryField invalid label={PACKAGE_FILE_INPUT_STORY_LABEL}>
      {(controlProps) => <PackageFileInput {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const ErrorState: Story = {
  render: () => (
    <PackageFormStoryField
      errorText="Cover image is required."
      label={PACKAGE_FILE_INPUT_STORY_LABEL}
    >
      {(controlProps) => <PackageFileInput {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const HelperText: Story = {
  render: () => (
    <PackageFormStoryField
      helperText="Shown on exported factory cards."
      label={PACKAGE_FILE_INPUT_STORY_LABEL}
    >
      {(controlProps) => <PackageFileInput {...controlProps} />}
    </PackageFormStoryField>
  ),
};

export const SelectedFile: Story = {
  render: () => <SelectedFileInputStoryExample />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = canvas.getByLabelText(PACKAGE_FILE_INPUT_STORY_LABEL);
    const file = new File(["cover"], "cover.png", { type: "image/png" });
    await userEvent.upload(input, file);
  },
};

export const Focus: Story = {
  render: () => (
    <PackageFormStoryField label={PACKAGE_FILE_INPUT_STORY_LABEL}>
      {(controlProps) => <PackageFileInput {...controlProps} autoFocus />}
    </PackageFormStoryField>
  ),
};

export const MobileWidth: Story = {
  decorators: [withMobileWidth],
  render: () => (
    <PackageFormStoryField
      helperText="Rendered at a 320px host width."
      label={PACKAGE_FILE_INPUT_STORY_LABEL}
    >
      {(controlProps) => <PackageFileInput {...controlProps} />}
    </PackageFormStoryField>
  ),
};
