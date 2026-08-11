import { expect, userEvent, within } from "storybook/test";
import type { PackagedFactoryCatalogResponse } from "../../../api/packaged-factories";
import { PackagedFactoryInventory } from "./packaged-factory-inventory";

const entry = (slug: string) => ({
  description: {
    type: "LOCALIZABLE_ASSET" as const,
    value: `${slug} Packaged Factory`,
  },
  examples: [
    {
      name: "default",
      description: {
        type: "LOCALIZABLE_ASSET" as const,
        value: `Run ${slug}`,
      },
      args: { input: `Run ${slug}` },
    },
  ],
  json: { id: `builtin-${slug}`, name: slug },
  name: `@you/${slug}`,
  project: `builtin-${slug}`,
  slug,
  yaml: `id: builtin-${slug}\nname: ${slug}\n`,
});

const readyCatalog: PackagedFactoryCatalogResponse = {
  factories: [entry("alpha"), entry("beta")],
};

function catalogParameters(body: PackagedFactoryCatalogResponse) {
  return {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/packaged-factories",
          response: { body },
        },
      ],
    },
  };
}

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
  parameters: catalogParameters(readyCatalog),
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
  parameters: catalogParameters({ factories: [] }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expect(
      within(canvasElement).findByText("No Packaged Factories are available."),
    ).resolves.toBeVisible();
  },
};

export const InvalidContract = {
  parameters: {
    dashboardApi: {
      fetchMocks: [
        {
          method: "GET",
          path: "/packaged-factories",
          response: { body: { factories: [{ invalid: true }] } },
        },
      ],
    },
  },
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    await expect(
      within(canvasElement).findByRole("alert"),
    ).resolves.toHaveTextContent(
      "The Packaged Factory catalog is unavailable.",
    );
  },
};

export const SelectedArtifactFailureRecovery = {
  parameters: catalogParameters({
    factories: [entry("alpha"), { ...entry("beta"), yaml: "" }],
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.findByRole("heading", { level: 3, name: "@you/alpha" }),
    ).resolves.toBeVisible();
    await userEvent.click(
      await canvas.findByRole("button", { name: /@you\/beta/ }),
    );
    await expect(canvas.findByRole("alert")).resolves.toHaveTextContent(
      "This Factory could not be loaded. Select another Factory to continue.",
    );
  },
};

export const MissingMetadata = {
  parameters: catalogParameters({
    factories: [{ ...entry("alpha"), examples: [] }],
  }),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
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
  },
  parameters: catalogParameters(readyCatalog),
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
