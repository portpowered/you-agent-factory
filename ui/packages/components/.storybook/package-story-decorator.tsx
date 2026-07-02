import type { Decorator } from "@storybook/react-vite";

export const withPackageStoryStyles: Decorator = (Story) => (
  <div data-color-palette="factory-dark" style={{ padding: "1rem" }}>
    <Story />
  </div>
);
