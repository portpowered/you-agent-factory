import { describe, expect, it } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../session-routing";
import { normalizeFactorySessionList } from "./normalize-session-list";

const canonicalDefault = {
  factoryDir: "/workspace/factory",
  folderPath: "/workspace",
  id: "019e0000-0000-7000-8000-000000000042",
  isDefault: true,
  project: "workspace",
  target: { kind: "default" as const },
};

const secondary = {
  factoryDir: "/workspace/secondary",
  folderPath: "/workspace",
  id: "019e0000-0000-7000-8000-000000000043",
  isDefault: false,
  project: "secondary",
  target: { kind: "named" as const, name: "secondary" },
};

describe("normalizeFactorySessionList", () => {
  it("keeps one canonical row when the default selector and UUID both arrive", () => {
    const result = normalizeFactorySessionList([
      { ...canonicalDefault, id: DEFAULT_FACTORY_SESSION_ID },
      canonicalDefault,
      secondary,
      { ...secondary },
    ]);

    expect(result).toEqual([canonicalDefault, secondary]);
    expect(result.map((session) => session.id)).not.toContain(
      DEFAULT_FACTORY_SESSION_ID,
    );
  });

  it("drops alias-only and blank rows instead of inventing session identities", () => {
    expect(
      normalizeFactorySessionList([
        { ...canonicalDefault, id: DEFAULT_FACTORY_SESSION_ID },
        { ...secondary, id: "  " },
      ]),
    ).toEqual([]);
  });

  it("normalizes surrounding whitespace without using labels as identity", () => {
    expect(
      normalizeFactorySessionList([
        { ...secondary, id: "  019e0000-0000-7000-8000-000000000043  " },
      ]),
    ).toEqual([secondary]);
  });
});
