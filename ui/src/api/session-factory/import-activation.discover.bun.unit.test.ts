import { beforeEach, describe, expect, it, mock } from "bun:test";

import { bunVi as vi } from "../../testing/bun/vi-compat";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();

mock.module("../factory-sessions", () => ({
  listFactorySessions: (...args: unknown[]) => listFactorySessions(...args),
  openFactorySession: (...args: unknown[]) => openFactorySession(...args),
}));

const { discoverSessionNamedFactoryNames } = await import("./import-activation");

describe("discoverSessionNamedFactoryNames", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
  });

  it("returns an empty list when the session has no folder path", async () => {
    listFactorySessions.mockResolvedValue([
      { id: "~default", folderPath: null },
      { id: "session-review", folderPath: "" },
    ]);

    await expect(
      discoverSessionNamedFactoryNames({ sessionID: "session-review" }),
    ).resolves.toEqual([]);

    expect(openFactorySession).not.toHaveBeenCalled();
  });

  it("opens the session folder and returns sorted named factory targets", async () => {
    listFactorySessions.mockResolvedValue([
      {
        id: "session-review",
        folderPath: "/tmp/factory-session",
      },
    ]);
    openFactorySession.mockResolvedValue({
      targets: [
        { ref: { kind: "named", name: "gamma" } },
        { ref: { kind: "named", name: "alpha" } },
        { ref: { kind: "bundled", path: "ignored" } },
      ],
    });

    await expect(
      discoverSessionNamedFactoryNames({ sessionID: "session-review" }),
    ).resolves.toEqual(["alpha", "gamma"]);

    expect(openFactorySession).toHaveBeenCalledWith(
      {
        folderPath: "/tmp/factory-session",
        validateOnly: true,
      },
      { fetch: expect.any(Function) },
    );
  });

  it("discovers names for the default session when sessionID is omitted", async () => {
    listFactorySessions.mockResolvedValue([
      {
        id: "~default",
        folderPath: "/tmp/default-session",
      },
    ]);
    openFactorySession.mockResolvedValue({
      targets: [{ ref: { kind: "named", name: "alpha" } }],
    });

    await expect(discoverSessionNamedFactoryNames()).resolves.toEqual([
      "alpha",
    ]);
  });
});
