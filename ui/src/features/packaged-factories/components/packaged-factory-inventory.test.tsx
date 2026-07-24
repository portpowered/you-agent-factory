import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { axe } from "jest-axe";

import {
  type PackagedFactoryPublicDataSource,
  type PackagedFactoryPublicExport,
  packagedFactoryManifestExport,
  packagedFactorySchemaExport,
} from "../lib/public-contract";
import { PackagedFactoryInventory } from "./packaged-factory-inventory";

const schemaIdentity =
  "https://schemas.portpowered.com/you/config/factory.schema.json";
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

function manifestEntry(slug: string, description: string, localized?: string) {
  return {
    description: {
      type: "LOCALIZABLE_ASSET",
      value: description,
      values: localized ? { "zh-CN": localized } : undefined,
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
  };
}

function manifest(factories: unknown[]) {
  return {
    factories,
    factorySchema: schemaIdentity,
    formatVersion: "1",
  };
}

function source({
  factories = [
    manifestEntry("alpha", "Alpha description", "Alpha 中文说明"),
    manifestEntry("beta", "Beta description"),
  ],
  overrides = {},
}: {
  factories?: unknown[];
  overrides?: Partial<Record<PackagedFactoryPublicExport, unknown>>;
} = {}): PackagedFactoryPublicDataSource {
  const values: Partial<Record<PackagedFactoryPublicExport, unknown>> = {
    [packagedFactoryManifestExport]: manifest(factories),
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

describe("PackagedFactoryInventory catalog states", () => {
  it("shows loading, empty, unsupported-version, and invalid-contract states without stale content", async () => {
    const neverResolves: PackagedFactoryPublicDataSource = {
      read: () => new Promise(() => {}),
    };
    const { rerender } = render(
      <PackagedFactoryInventory source={neverResolves} />,
    );
    expect(screen.getByText("Loading Packaged Factories…")).not.toBeNull();

    rerender(
      <PackagedFactoryInventory
        source={source({
          factories: [],
        })}
      />,
    );
    expect(
      await screen.findByText("No Packaged Factories are available."),
    ).not.toBeNull();

    rerender(
      <PackagedFactoryInventory
        source={source({
          overrides: {
            [packagedFactoryManifestExport]: {
              ...manifest([]),
              formatVersion: "2",
            },
          },
        })}
      />,
    );
    expect((await screen.findByRole("alert")).textContent).toContain(
      "This website does not support Packaged Factory catalog format 2.",
    );

    rerender(
      <PackagedFactoryInventory
        source={source({
          overrides: { [packagedFactoryManifestExport]: { factories: [] } },
        })}
      />,
    );
    expect((await screen.findByRole("alert")).textContent).toContain(
      "The Packaged Factory catalog is unavailable.",
    );
    expect(screen.queryByText("@you/alpha")).toBeNull();
  });
});

describe("PackagedFactoryInventory selection", () => {
  it("renders every stable name once and supports arrow traversal and keyboard activation", async () => {
    const user = userEvent.setup();
    const { baseElement } = render(
      <PackagedFactoryInventory source={source()} />,
    );

    const alpha = await screen.findByRole("button", { name: /@you\/alpha/ });
    const beta = screen.getByRole("button", {
      name: /@you\/beta/,
    });
    expect(screen.getAllByText("@you/alpha")).toHaveLength(2);
    expect(alpha.getAttribute("aria-current")).toBe("true");
    expect(alpha.getAttribute("aria-pressed")).toBe("true");

    alpha.focus();
    fireEvent.keyDown(alpha, { key: "ArrowDown" });
    expect(document.activeElement).toBe(beta);
    await user.keyboard("{Enter}");

    await waitFor(() => {
      expect(beta.getAttribute("aria-current")).toBe("true");
    });
    expect(
      await screen.findByRole("heading", {
        level: 3,
        name: "@you/beta",
      }),
    ).not.toBeNull();
    expect(
      screen.getByText("@you/beta selected").getAttribute("aria-live"),
    ).toBe("polite");
    expect((await axe(baseElement)).violations).toEqual([]);
  });

  it("contains selected-artifact failures and recovers through another selection", async () => {
    const user = userEvent.setup();
    render(
      <PackagedFactoryInventory
        source={source({
          overrides: {
            "@you-agent-factory/packaged-factories/factories/alpha.json":
              undefined,
          },
        })}
      />,
    );

    expect((await screen.findByRole("alert")).textContent).toContain(
      "This Factory could not be loaded. Select another Factory to continue.",
    );
    expect(
      screen
        .getByRole("button", { name: /@you\/alpha/ })
        .getAttribute("aria-current"),
    ).toBe("true");
    expect(
      screen.queryByRole("heading", { level: 3, name: "@you/alpha" }),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: /@you\/beta/ }));

    expect(
      await screen.findByRole("heading", { level: 3, name: "@you/beta" }),
    ).not.toBeNull();
    expect(
      screen.queryByText(
        "This Factory could not be loaded. Select another Factory to continue.",
      ),
    ).toBeNull();
  });

  it("uses localized copy and renders package content as inert escaped text", async () => {
    const hostile = "<img src=x onerror=alert(1)>";
    render(
      <PackagedFactoryInventory
        locale="zh-CN"
        source={source({
          factories: [manifestEntry("alpha", hostile, hostile)],
        })}
      />,
    );

    expect(
      await screen.findByRole("navigation", { name: "可用的打包工厂" }),
    ).not.toBeNull();
    expect(screen.getAllByText(hostile).length).toBeGreaterThan(0);
    expect(document.querySelector("img")).toBeNull();
  });
});
