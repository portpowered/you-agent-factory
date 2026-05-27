import { getFactoryPngImportMessages } from "./factory-png-import";

describe("getFactoryPngImportMessages", () => {
  it("returns default import PNG errors", () => {
    const messages = getFactoryPngImportMessages();

    expect(messages.notPngFile).toBe("The selected file is not a PNG image.");
    expect(messages.factoryMetadataInvalidJson).toBe(
      "The you-agent-factory factory metadata is not valid JSON.",
    );
    expect(messages.unsupportedSchemaVersion).toBe(
      "The selected PNG uses an unsupported you-agent-factory factory metadata version.",
    );
  });

  it("returns zh-CN import PNG errors", () => {
    const messages = getFactoryPngImportMessages("zh-CN");

    expect(messages.notPngFile).toBe("所选文件不是 PNG 图片。");
    expect(messages.factoryMetadataInvalidJson).toBe(
      "you-agent-factory 工厂元数据不是有效的 JSON。",
    );
    expect(messages.unsupportedSchemaVersion).toBe(
      "所选 PNG 使用了不受支持的 you-agent-factory 工厂元数据版本。",
    );
  });
});
