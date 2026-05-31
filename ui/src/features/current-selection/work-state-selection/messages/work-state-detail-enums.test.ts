import { describe, expect, it } from "vitest";

import { localizeWorkStateType } from "./work-state-detail-enums";

describe("localizeWorkStateType", () => {
  it("localizes work state lifecycle types for supported locales", () => {
    expect(localizeWorkStateType("INITIAL", "en")).toBe("Initial");
    expect(localizeWorkStateType("PROCESSING", "ja")).toBe("処理中");
    expect(localizeWorkStateType("TERMINAL", "ko")).toBe("완료");
    expect(localizeWorkStateType("FAILED", "zh-CN")).toBe("失败");
  });
});
