import {
  clampPromptEditorResizeHeight,
  PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX,
  PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX,
} from "./prompt-editor-resize-bounds";

describe("prompt-editor-resize-bounds", () => {
  it("clamps height to configured min and max bounds", () => {
    expect(
      clampPromptEditorResizeHeight(120, {
        minHeight: PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX,
        maxHeight: PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX,
      }),
    ).toBe(PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX);
    expect(
      clampPromptEditorResizeHeight(1200, {
        minHeight: PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX,
        maxHeight: PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX,
      }),
    ).toBe(PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX);
    expect(
      clampPromptEditorResizeHeight(420.6, {
        minHeight: PROMPT_EDITOR_RESIZE_MIN_HEIGHT_PX,
        maxHeight: PROMPT_EDITOR_RESIZE_MAX_HEIGHT_PX,
      }),
    ).toBe(421);
  });
});
