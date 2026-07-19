import type { Preview } from "@storybook/react-vite";

import "@you-agent-factory/components/styles.css";
import "@xyflow/react/dist/style.css";
import "../src/styles.css";

const preview: Preview = {
  decorators: [
    (Story) => (
      <div data-color-palette="factory-dark">
        <Story />
      </div>
    ),
  ],
  parameters: {
    layout: "padded",
  },
};

export default preview;
