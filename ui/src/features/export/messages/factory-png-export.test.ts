import { getFactoryPngExportMessages } from "./factory-png-export";

describe("getFactoryPngExportMessages", () => {
  it("returns default export PNG errors", () => {
    const messages = getFactoryPngExportMessages();

    expect(messages.imageDecodeFailed).toBe(
      "The selected image could not be decoded for PNG export.",
    );
    expect(messages.metadataWriteFailed).toBe(
      "The exported PNG metadata could not be written.",
    );
  });

  it("returns zh-CN export PNG errors", () => {
    const messages = getFactoryPngExportMessages("zh-CN");

    expect(messages.imageDecodeFailed).toBe("无法解码所选图片以导出 PNG。");
    expect(messages.metadataWriteFailed).toBe("无法写入导出的 PNG 元数据。");
  });
});
