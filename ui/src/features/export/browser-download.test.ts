import { downloadBlobAsFile } from "./browser-download";

describe("downloadBlobAsFile", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("creates a temporary download anchor and cleans up the object URL", () => {
    const createObjectURL = vi.fn(() => "blob:factory-download");
    const revokeObjectURL = vi.fn();
    const clickSpy = vi
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {});
    vi.stubGlobal("URL", {
      ...URL,
      createObjectURL,
      revokeObjectURL,
    });

    downloadBlobAsFile({
      blob: new Blob(["png"], { type: "image/png" }),
      filename: "factory-aurora.png",
    });

    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(clickSpy).toHaveBeenCalledTimes(1);
    expect(document.querySelector("a[download='factory-aurora.png']")).toBeNull();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:factory-download");
  });
});
