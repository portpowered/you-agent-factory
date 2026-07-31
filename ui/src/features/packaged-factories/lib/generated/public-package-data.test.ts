import { describe, expect, it } from "vitest";

import { createPackagedFactoryPublicValues } from "./public-package-data";

describe("packaged Factory HTTP catalog projection", () => {
  it("preserves descriptions and examples in the dashboard manifest", () => {
    const description = {
      type: "LOCALIZABLE_ASSET" as const,
      value: "Runs a useful packaged workflow.",
    };
    const examples = [
      {
        name: "default",
        description: {
          type: "LOCALIZABLE_ASSET" as const,
          value: "Run the workflow.",
        },
        args: { request: "do the work" },
      },
    ];
    const values = createPackagedFactoryPublicValues({
      factories: [
        {
          name: "@you/example",
          project: "builtin-example",
          slug: "example",
          description,
          examples,
          json: { name: "@you/example", description, examples },
          yaml: "name: '@you/example'\n",
        },
      ],
    });

    expect(
      values.get("@you-agent-factory/packaged-factories/manifest"),
    ).toMatchObject({
      factories: [{ name: "@you/example", description, examples }],
    });
  });
});
