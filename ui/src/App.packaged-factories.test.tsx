import "@testing-library/jest-dom/vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";
import { App, PACKAGED_FACTORIES_HOSTED_PATH } from "./App";
import type {
  PackagedFactoryPublicDataSource,
  PackagedFactoryPublicExport,
} from "./features/packaged-factories/public";

const packagePrefix = "@you-agent-factory/packaged-factories";
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
const entry = (slug: string) => ({
  description: {
    type: "LOCALIZABLE_ASSET",
    value: `${slug} base description`,
    values: { "zh-CN": `${slug} 中文说明` },
  },
  examples: [
    {
      args: { topic: `<script>${slug}</script>` },
      description: {
        type: "LOCALIZABLE_ASSET",
        value: `${slug} example`,
      },
      name: `${slug}-example`,
    },
  ],
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

function appSource(): PackagedFactoryPublicDataSource {
  const values: Partial<Record<PackagedFactoryPublicExport, unknown>> = {
    [`${packagePrefix}/manifest`]: {
      factories: [entry("alpha"), entry("beta")],
      factorySchema: schemaIdentity,
      formatVersion: "1",
    },
    [`${packagePrefix}/schemas/factory.json`]: schema,
    [`${packagePrefix}/factories/alpha.json`]:
      '{"id":"builtin-alpha","name":"alpha"}',
    [`${packagePrefix}/factories/alpha.yaml`]:
      "id: builtin-alpha\nname: alpha\n",
    [`${packagePrefix}/factories/beta.json`]: undefined,
    [`${packagePrefix}/factories/beta.yaml`]: "id: builtin-beta\nname: beta\n",
  };
  return {
    async read(specifier) {
      return values[specifier];
    },
  };
}

describe("Packaged Factories app route", () => {
  afterEach(() => {
    window.history.replaceState({}, "", "/");
  });

  it("discovers, localizes, inspects, copies, and recovers through the shipped app path", async () => {
    const user = userEvent.setup();
    const copyText = vi.fn().mockResolvedValue(undefined);
    window.history.pushState({}, "", PACKAGED_FACTORIES_HOSTED_PATH);

    render(
      <App
        initialLocale="zh-CN"
        packagedFactoryCopyText={copyText}
        packagedFactorySource={appSource()}
      />,
    );

    expect(
      await screen.findByRole("navigation", { name: "可用的打包工厂" }),
    ).toBeVisible();
    expect(screen.getAllByText("alpha 中文说明")).toHaveLength(2);

    await user.click(screen.getByRole("button", { name: "YAML" }));
    await user.click(screen.getByRole("button", { name: "复制 YAML 配置" }));
    expect(copyText).toHaveBeenLastCalledWith(
      "id: builtin-alpha\nname: alpha\n",
    );

    await user.click(
      screen.getByRole("button", {
        name: "复制 alpha-example 调用示例",
      }),
    );
    expect(copyText).toHaveBeenLastCalledWith(
      expect.stringContaining("<script>alpha</script>"),
    );
    expect(document.querySelector("script")).toBeNull();

    await user.click(screen.getByRole("button", { name: /@you\/beta/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "无法加载此工厂。请选择其他工厂以继续。",
    );
    expect(
      screen.queryByRole("heading", { level: 3, name: "@you/beta" }),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: /@you\/alpha/ }));
    await waitFor(() => {
      expect(
        screen.getByRole("heading", { level: 3, name: "@you/alpha" }),
      ).toBeVisible();
    });
  });
});
