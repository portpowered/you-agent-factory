import {
  FactoryPreviewAPIError,
  factoryPreviewAPIErrorMessages,
  previewFactory,
} from "./index";

describe("factory-preview public exports", () => {
  it("re-exports the canonical factory preview API surface", () => {
    expect(previewFactory).toBeTypeOf("function");
    expect(FactoryPreviewAPIError).toBeTypeOf("function");
    expect(factoryPreviewAPIErrorMessages.network).toBeTruthy();
  });
});
