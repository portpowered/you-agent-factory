// @vitest-environment node

import type { components } from "../../api/generated/openapi";
import { writeFactoryExportPng } from "./factory-png-export";

type FactorySchemas = components["schemas"];

const canonicalFactory: FactorySchemas["Factory"] = {
  id: "agent-factory",
  name: "agent-factory",
  workTypes: [],
  workers: [],
  workstations: [],
};

describe("writeFactoryExportPng outside browser decoding contexts", () => {
  it("returns an explicit decode failure when no browser image decoder is available", async () => {
    const result = await writeFactoryExportPng({
      factory: canonicalFactory,
      image: new Blob(["png"], { type: "image/png" }),
    });

    expect(result).toEqual({
      error: {
        cause: expect.any(Error),
        code: "IMAGE_DECODE_FAILED",
        message: "The selected image could not be decoded for PNG export.",
      },
      ok: false,
    });
  });
});
