import { expect, userEvent, within } from "storybook/test";

import {
  type PackagedFactoryPublicDataSource,
  type PackagedFactoryPublicExport,
  packagedFactoryManifestExport,
  packagedFactorySchemaExport,
} from "../lib/public-contract";
import { PackagedFactoryInventory } from "./packaged-factory-inventory";

const schemaIdentity =
  "https://schemas.portpowered.com/you/config/factory.schema.json";
const entry = (slug: string) => ({
  description: {
    type: "LOCALIZABLE_ASSET",
    value: `${slug} Packaged Factory`,
  },
  json: {
    locator: `generated/factories/${slug}/factory.json`,
    sha256: "a".repeat(64),
  },
  name: `@you/${slug}`,
  project: `builtin-${slug}`,
  slug,
  yaml: {
    locator: `generated/factories/${slug}/factory.yaml`,
    sha256: "b".repeat(64),
  },
});
const entryWithoutDescription = (slug: string) => {
  const { description: _description, ...value } = entry(slug);
  return value;
};
const schema = {
  $id: schemaIdentity,
  $schema: "https://json-schema.org/draft/2020-12/schema",
  additionalProperties: false,
  properties: {
    id: { type: "string" },
    name: { type: "string" },
  },
  required: ["id", "name"],
  type: "object",
};

function storySource(
  manifest: unknown,
  overrides: Partial<Record<PackagedFactoryPublicExport, unknown>> = {},
): PackagedFactoryPublicDataSource {
  const values: Partial<Record<PackagedFactoryPublicExport, unknown>> = {
    [packagedFactoryManifestExport]: manifest,
    [packagedFactorySchemaExport]: schema,
    "@you-agent-factory/packaged-factories/factories/alpha.json":
      '{"id":"builtin-alpha","name":"alpha"}',
    "@you-agent-factory/packaged-factories/factories/alpha.yaml":
      "id: builtin-alpha\nname: alpha\n",
    "@you-agent-factory/packaged-factories/factories/beta.json":
      '{"id":"builtin-beta","name":"beta"}',
    "@you-agent-factory/packaged-factories/factories/beta.yaml":
      "id: builtin-beta\nname: beta\n",
    ...overrides,
  };
  return {
    async read(specifier) {
      return values[specifier];
    },
  };
}

const readyManifest = {
  factories: [entry("alpha"), entry("beta")],
  factorySchema: schemaIdentity,
  formatVersion: "1",
};

export default {
  component: PackagedFactoryInventory,
  decorators: [
    (Story: () => React.ReactNode) => (
      <div className="mx-auto min-w-0 max-w-7xl p-4">
        <Story />
      </div>
    ),
  ],
  tags: ["test"],
  title: "Agent Factory/Packaged Factories/Inventory",
};

export const ReadyResponsive = {
  args: { source: storySource(readyManifest) },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const beta = await canvas.findByRole("button", { name: /@you\/beta/ });
    await userEvent.click(beta);
    await expect(
      canvas.findByRole("heading", { level: 3, name: "@you/beta" }),
    ).resolves.toBeVisible();
    await userEvent.click(canvas.getByRole("button", { name: "YAML" }));
    await expect(canvasElement.querySelector("code")).toHaveTextContent(
      "id: builtin-beta name: beta",
    );
    await expect(canvas.getByLabelText("Packaged Factory catalog")).toHaveClass(
      "min-w-0",
    );
  },
};

export const Empty = {
  args: {
    source: storySource({
      ...readyManifest,
      factories: [],
    }),
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expect(
      within(canvasElement).findByText("No Packaged Factories are available."),
    ).resolves.toBeVisible();
  },
};

export const InvalidContract = {
  args: { source: storySource({ factories: [] }) },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expect(
      within(canvasElement).findByRole("alert"),
    ).resolves.toHaveTextContent(
      "The Packaged Factory catalog is unavailable.",
    );
  },
};

export const SelectedArtifactFailureRecovery = {
  args: {
    source: storySource(readyManifest, {
      "@you-agent-factory/packaged-factories/factories/alpha.json": undefined,
    }),
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.findByRole("alert")).resolves.toBeVisible();
    await userEvent.click(
      await canvas.findByRole("button", { name: /@you\/beta/ }),
    );
    await expect(
      canvas.findByRole("heading", { level: 3, name: "@you/beta" }),
    ).resolves.toBeVisible();
  },
};

export const MissingMetadata = {
  args: {
    source: storySource({
      ...readyManifest,
      factories: [entryWithoutDescription("alpha")],
    }),
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.findAllByText("Description unavailable"),
    ).resolves.toHaveLength(2);
    await expect(
      canvas.findByText("No invocation examples are available."),
    ).resolves.toBeVisible();
  },
};

export const CopyFailure = {
  args: {
    copyText: async () => {
      throw new Error("Clipboard permission denied.");
    },
    source: storySource(readyManifest),
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Copy JSON configuration",
      }),
    );
    await expect(canvas.findByRole("alert")).resolves.toHaveTextContent(
      "Could not copy configuration. Try again.",
    );
  },
};
