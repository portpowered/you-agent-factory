import { renderHook } from "@testing-library/react";

import {
  CurrentFactoryDefinitionError,
  saveFactoryForSessionDocument,
} from "../../../api/current-factory-definition";
import {
  createFactoryDocumentSaveQueryClientWrapper,
  editableFactoryDefinition,
} from "./useFactoryDocumentSave.test-helpers";
import { useFactoryDocumentSave } from "./useFactoryDocumentSave";

vi.mock("../../../api/current-factory-definition", async () => {
  const actual = await vi.importActual(
    "../../../api/current-factory-definition",
  );

  return {
    ...actual,
    saveFactoryForSessionDocument: vi.fn(),
  };
});

beforeEach(() => {
  vi.mocked(saveFactoryForSessionDocument).mockReset();
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
    vi.mocked(saveFactoryForSessionDocument).mockRejectedValue(error);

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
