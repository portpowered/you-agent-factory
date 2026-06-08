import { describe, expect, it } from "vitest";

import { validateFactoryGraphAddModelOperationsDraft } from "./factory-graph-add-model-operation-draft";

describe("factory graph add model operation draft validation", () => {
  it("reports empty operation names and missing slot lists", () => {
    expect(
      validateFactoryGraphAddModelOperationsDraft([
        {
          inputs: [],
          name: "   ",
          outputs: [],
        },
      ]),
    ).toMatchObject({
      byIndex: {
        0: {
          inputs: "Add at least one input slot for each operation.",
          name: "Enter an uppercase operation name.",
          outputs: "Add at least one output slot for each operation.",
        },
      },
    });
  });
});
