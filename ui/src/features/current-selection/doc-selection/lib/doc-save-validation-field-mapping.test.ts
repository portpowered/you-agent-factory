import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { mapDocSaveErrorToFieldErrors } from "./doc-save-validation-field-mapping";

describe("mapDocSaveErrorToFieldErrors", () => {
  it("maps bundled doc path validation targets onto fileName", () => {
    const error = new CurrentFactoryDefinitionError("Invalid doc path.", {
      targets: [
        {
          code: "bundled-file-target-path",
          message: "Doc paths must stay under factory/docs/.",
        },
      ],
    });

    expect(mapDocSaveErrorToFieldErrors(error)).toEqual({
      fileName: "Doc paths must stay under factory/docs/.",
    });
  });

  it("maps bundled doc content validation targets onto inlineContent", () => {
    const error = new CurrentFactoryDefinitionError("Invalid doc content.", {
      targets: [
        {
          code: "bundled-file-content-inline",
          message: "Doc content must be UTF-8 text.",
        },
      ],
    });

    expect(mapDocSaveErrorToFieldErrors(error)).toEqual({
      inlineContent: "Doc content must be UTF-8 text.",
    });
  });

  it("returns undefined when no doc field targets are present", () => {
    const error = new CurrentFactoryDefinitionError("Unrelated failure.", {
      targets: [
        {
          code: "worker-timeout",
          message: "Invalid timeout.",
          subject: { id: "writer", type: "WORKER" },
        },
      ],
    });

    expect(mapDocSaveErrorToFieldErrors(error)).toBeUndefined();
  });
});
