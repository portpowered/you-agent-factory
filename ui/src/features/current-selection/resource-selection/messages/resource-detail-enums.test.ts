import { getResourceDetailEnumMessages } from "./resource-detail-enums";

describe("getResourceDetailEnumMessages", () => {
  it("localizes resource types for supported locales", () => {
    expect(getResourceDetailEnumMessages().localizeResourceType("MODEL")).toBe(
      "Model",
    );
    expect(
      getResourceDetailEnumMessages("ja").localizeResourceType(
        "INVOCATION_SLOT",
      ),
    ).toBe("呼び出しスロット");
    expect(
      getResourceDetailEnumMessages("zh-CN").localizeResourceType(
        "PROVIDER_QUOTA",
      ),
    ).toBe("提供商配额");
  });
});
