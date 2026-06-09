import {
  WorkflowPreviewAPIError,
  previewWorkflow,
  workflowPreviewAPIErrorMessages,
} from "./index";

describe("workflow-preview public exports", () => {
  it("re-exports the preview API surface", () => {
    expect(previewWorkflow).toBeTypeOf("function");
    expect(WorkflowPreviewAPIError).toBeTypeOf("function");
    expect(workflowPreviewAPIErrorMessages.network).toBeTruthy();
  });
});
