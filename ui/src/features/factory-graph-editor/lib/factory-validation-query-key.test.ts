import { describe, expect, it } from "vitest";

import { baseFactoryDefinition } from "./factory-graph-draft.test-helpers";
import { serializeFactoryValidationDefinition } from "./factory-validation-query-key";

describe("serializeFactoryValidationDefinition", () => {
  it("returns null for a missing factory definition", () => {
    expect(serializeFactoryValidationDefinition(null)).toBeNull();
  });

  it("returns stable serialized keys for equivalent factory definitions", () => {
    const first = serializeFactoryValidationDefinition(baseFactoryDefinition);
    const second = serializeFactoryValidationDefinition(
      structuredClone(baseFactoryDefinition),
    );

    expect(first).toBe(second);
    expect(first).toEqual(expect.any(String));
  });
});
