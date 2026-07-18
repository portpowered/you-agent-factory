import { describe, expect, expectTypeOf, it } from "vitest";

import {
  type components,
  FACTORY_EVENT_TYPES,
  type FactoryDefinition,
  type FactoryEvent,
  type FactoryEventType,
  type operations,
  type paths,
} from "./index.js";

describe("public Factory contracts", () => {
  it("exposes generated event constants through the stable client boundary", () => {
    expect(FACTORY_EVENT_TYPES.FactoryEventTypeRunRequest).toBe("RUN_REQUEST");
    expect(FACTORY_EVENT_TYPES.FactoryEventTypeArtifactCreated).toBe(
      "ARTIFACT_CREATED",
    );
    expect(Object.values(FACTORY_EVENT_TYPES)).toContain("FACTORY_CHANGE");
  });

  it("keeps the public aliases compatible with the generated namespaces", () => {
    expectTypeOf<FactoryEvent>().toEqualTypeOf<
      components["schemas"]["FactoryEvent"]
    >();
    expectTypeOf<FactoryEventType>().toEqualTypeOf<
      components["schemas"]["FactoryEventType"]
    >();
    expectTypeOf<FactoryDefinition>().toEqualTypeOf<
      components["schemas"]["Factory"]
    >();
    expectTypeOf<paths>().toBeObject();
    expectTypeOf<operations>().toBeObject();
  });

  it("types every published runtime constant as a supported event type", () => {
    expectTypeOf(Object.values(FACTORY_EVENT_TYPES)).toMatchTypeOf<
      FactoryEventType[]
    >();
  });
});
