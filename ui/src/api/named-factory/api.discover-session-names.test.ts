import { beforeEach, describe, expect, it, vi } from "vitest";

import { discoverSessionNamedFactoryNames } from "./api";

const listFactorySessions = vi.fn();
const openFactorySession = vi.fn();

vi.mock("../factory-sessions", () => ({
  listFactorySessions: (...args: unknown[]) => listFactorySessions(...args),
  openFactorySession: (...args: unknown[]) => openFactorySession(...args),
}));

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
});
