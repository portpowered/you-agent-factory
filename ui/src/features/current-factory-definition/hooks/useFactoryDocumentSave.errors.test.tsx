import "../../../../testing/bun-current-factory-definition-api-mocks";
import { renderHook } from "@testing-library/react";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import { saveFactoryForSessionDocumentMock } from "../../../../testing/bun-current-factory-definition-api-mocks";
import {
  createFactoryDocumentSaveQueryClientWrapper,
  editableFactoryDefinition,
} from "./useFactoryDocumentSave.test-helpers";
import { useFactoryDocumentSave } from "./useFactoryDocumentSave";

beforeEach(() => {
  saveFactoryForSessionDocumentMock.mockReset();
});

describe("useFactoryDocumentSave operator errors", () => {
  it.each([
    {
      code: "STALE_FACTORY_VERSION" as const,
      status: 409,
    },
    {
      code: "FACTORY_NOT_IDLE" as const,
    },
    {
      code: "INVALID_FACTORY_DEFINITION" as const,
    },
  ])("surfaces $code through saveAsync rejections", async ({ code, status }) => {
    const error = new CurrentFactoryDefinitionError(
      `Save failed with ${code}.`,
      {
        code,
        status,
      },
    );
    saveFactoryForSessionDocumentMock.mockRejectedValue(error);

    const { result } = renderHook(() => useFactoryDocumentSave(), {
      wrapper: createFactoryDocumentSaveQueryClientWrapper(),
    });

    await expect(
      result.current.saveAsync({
        factory: editableFactoryDefinition,
      }),
    ).rejects.toMatchObject({
      code,
      name: "CurrentFactoryDefinitionError",
    });
  });
});
