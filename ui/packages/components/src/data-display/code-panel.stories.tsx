import type { Meta, StoryObj } from "@storybook/react-vite";

import { CodePanel } from "./code-panel";

const LONG_SINGLE_LINE =
  'export const configuration = {"factoryDirectory":"/very/long/path/to/factory","metadata":{"factory_hash":"sha256:28cfd1d2e1c8f102239233ee3e6fc0002b2888d8746131af42e61c01eec21f56","runtime_config_hash":"sha256:593d2aa9ef0fe0519083808a0d37a54a8c6ef4255abdf1b7fcc4a11d4000b049"}};';

const LONG_MULTI_LINE = Array.from(
  { length: 24 },
  (_, index) =>
    `line ${index + 1}: worker output payload with diagnostic context and repeated tokens`,
).join("\n");

const meta = {
  title: "Data Display/CodePanel",
  component: CodePanel,
  parameters: {
    layout: "padded",
  },
} satisfies Meta<typeof CodePanel>;

export default meta;

type Story = StoryObj<typeof meta>;

export const ShortCode: Story = {
  args: {
    children: "const value = 1;",
  },
};

export const LongSingleLine: Story = {
  render: () => (
    <div className="w-full max-w-md">
      <CodePanel>{LONG_SINGLE_LINE}</CodePanel>
    </div>
  ),
};

export const LongMultiLine: Story = {
  render: () => (
    <div className="w-full max-w-md">
      <CodePanel maxHeight="md">{LONG_MULTI_LINE}</CodePanel>
    </div>
  ),
};

export const LongCode: Story = {
  parameters: {
    viewport: {
      defaultViewport: "mobile1",
    },
  },
  render: () => (
    <div className="grid w-full max-w-md gap-2">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <span className="min-w-0 shrink text-body-medium text-on-surface">
          Generated script
        </span>
        <button
          className="shrink-0 rounded-lg border border-outline bg-surface-container-high px-2 py-1 text-body-small text-on-surface focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
          type="button"
        >
          Copy
        </button>
      </div>
      <CodePanel maxHeight="md" padding="default" surface="low">
        {LONG_SINGLE_LINE}
        {"\n"}
        {LONG_MULTI_LINE}
      </CodePanel>
    </div>
  ),
};

export const DesktopLongCode: Story = {
  parameters: {
    viewport: {
      defaultViewport: "desktop",
    },
  },
  render: () => (
    <div className="grid w-full max-w-3xl gap-2">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <span className="min-w-0 shrink text-body-medium text-on-surface">
          Generated script
        </span>
        <button
          className="shrink-0 rounded-lg border border-outline bg-surface-container-high px-2 py-1 text-body-small text-on-surface focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
          type="button"
        >
          Copy
        </button>
      </div>
      <CodePanel maxHeight="lg" padding="default" surface="low">
        {LONG_SINGLE_LINE}
        {"\n"}
        {LONG_MULTI_LINE}
      </CodePanel>
    </div>
  ),
};
