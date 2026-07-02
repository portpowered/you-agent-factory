import type { Preview } from "@storybook/react-vite";

import { withPackageStoryStyles } from "./package-story-decorator";

import "../src/styles/package-token-styles-fixture.css";

const preview: Preview = {
  decorators: [withPackageStoryStyles],
  parameters: {
    layout: "centered",
  },
};

export default preview;
