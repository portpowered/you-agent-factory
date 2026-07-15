import type { Preview } from "@storybook/react-vite";

import "../src/styles.css";
import { withDashboardStoryRuntime } from "./dashboard-story-runtime";

const preview: Preview = {
  decorators: [withDashboardStoryRuntime],
  parameters: {
    layout: "fullscreen",
  },
};

export default preview;
