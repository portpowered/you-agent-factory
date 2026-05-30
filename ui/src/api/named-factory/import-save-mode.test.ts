import type { FactorySessionTarget } from "../factory-sessions";
import {
  allocateFirstFreeSuffixedFactoryName,
  extractNamedFactoryNamesFromSessionTargets,
  resolveImportCreateFactoryName,
} from "./import-save-mode";

describe("named factory import save mode helpers", () => {
  it("extracts sorted unique named factory names from session targets", () => {
    const targets: FactorySessionTarget[] = [
      {
        factoryDir: "/tmp/factories/beta",
        folderPath: "/tmp/factories",
        label: "Beta",
        project: "beta",
        ref: { kind: "named", name: "beta" },
      },
      {
        factoryDir: "/tmp/factories",
        folderPath: "/tmp/factories",
        label: "Default",
        project: "default",
        ref: { kind: "default" },
      },
      {
        factoryDir: "/tmp/factories/alpha",
        folderPath: "/tmp/factories",
        label: "Alpha",
        project: "alpha",
        ref: { kind: "named", name: "alpha" },
      },
      {
        factoryDir: "/tmp/factories/alpha",
        folderPath: "/tmp/factories",
        label: "Alpha duplicate",
        project: "alpha",
        ref: { kind: "named", name: "alpha" },
      },
    ];

    expect(extractNamedFactoryNamesFromSessionTargets(targets)).toEqual([
      "alpha",
      "beta",
    ]);
  });

  it("returns the preferred name when it is unused", () => {
    expect(
      allocateFirstFreeSuffixedFactoryName("Dropped Factory", ["alpha"]),
    ).toBe("Dropped Factory");
  });

  it("allocates the first free suffixed name when the preferred name exists", () => {
    expect(
      allocateFirstFreeSuffixedFactoryName("Dropped Factory", [
        "Dropped Factory",
      ]),
    ).toBe("Dropped Factory-2");
    expect(
      allocateFirstFreeSuffixedFactoryName("Dropped Factory", [
        "Dropped Factory",
        "Dropped Factory-2",
      ]),
    ).toBe("Dropped Factory-3");
  });

  it("returns an empty list when session targets are missing or unnamed", () => {
    expect(extractNamedFactoryNamesFromSessionTargets(undefined)).toEqual([]);
    expect(
      extractNamedFactoryNamesFromSessionTargets([
        {
          factoryDir: "/tmp/factories",
          folderPath: "/tmp/factories",
          label: "Default",
          project: "default",
          ref: { kind: "default" },
        },
        {
          factoryDir: "/tmp/factories/ ",
          folderPath: "/tmp/factories",
          label: "Blank",
          project: "blank",
          ref: { kind: "named", name: "   " },
        },
      ]),
    ).toEqual([]);
  });

  it("returns the preferred name unchanged when it is blank", () => {
    expect(allocateFirstFreeSuffixedFactoryName("   ", ["alpha"])).toBe("   ");
  });

  it("reports whether the resolved create target replaces an existing factory", () => {
    expect(
      resolveImportCreateFactoryName("Dropped Factory", ["alpha"]),
    ).toEqual({
      factoryName: "Dropped Factory",
      replacesExisting: false,
    });
    expect(
      resolveImportCreateFactoryName("Dropped Factory", ["Dropped Factory"]),
    ).toEqual({
      factoryName: "Dropped Factory-2",
      replacesExisting: false,
    });
    expect(
      resolveImportCreateFactoryName("Dropped Factory", [
        "Dropped Factory",
        "Dropped Factory-2",
      ]),
    ).toEqual({
      factoryName: "Dropped Factory-2",
      replacesExisting: true,
    });
    expect(
      resolveImportCreateFactoryName("Dropped Factory", [
        "Dropped Factory",
        "Dropped Factory-2",
        "Dropped Factory-3",
      ]),
    ).toEqual({
      factoryName: "Dropped Factory-4",
      replacesExisting: false,
    });
  });
});
