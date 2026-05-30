import { allocateImportCreateFactoryName } from "./allocate-import-create-factory-name";

describe("allocateImportCreateFactoryName", () => {
  it("returns the embedded name when it is not taken", () => {
    expect(allocateImportCreateFactoryName("Dropped Factory", ["alpha"])).toBe(
      "Dropped Factory",
    );
  });

  it("allocates the first free numeric suffix when the embedded name collides", () => {
    expect(
      allocateImportCreateFactoryName("Dropped Factory", ["Dropped Factory"]),
    ).toBe("Dropped Factory-2");
  });

  it("skips occupied suffixed names", () => {
    expect(
      allocateImportCreateFactoryName("Dropped Factory", [
        "Dropped Factory",
        "Dropped Factory-2",
      ]),
    ).toBe("Dropped Factory-3");
  });

  it("trims embedded and existing names before comparing", () => {
    expect(
      allocateImportCreateFactoryName("  Dropped Factory  ", ["  Dropped Factory  "]),
    ).toBe("Dropped Factory-2");
  });
});
