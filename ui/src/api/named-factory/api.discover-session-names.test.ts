import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  activateImportedFactoryAsNewNamedForSession,
  discoverSessionNamedFactoryNames,
} from "./api";
import { mockPutSessionFactory } from "../../testing/session-factory-mocks";

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

    await expect(discoverSessionNamedFactoryNames()).resolves.toEqual(["alpha"]);
  });
});

describe("activateImportedFactoryAsNewNamedForSession discovery", () => {
  beforeEach(() => {
    listFactorySessions.mockReset();
    openFactorySession.mockReset();
  });

  it("discovers existing names before UPSERT when none are provided", async () => {
    listFactorySessions.mockResolvedValue([
      {
        id: "~default",
        folderPath: "/tmp/default-session",
      },
    ]);
    openFactorySession.mockResolvedValue({
      targets: [{ ref: { kind: "named", name: "Imported Factory" } }],
    });

    const fetchMock = vi.fn().mockResolvedValueOnce(
      mockPutSessionFactory({
        responseDocument: {
          name: "Imported Factory-2",
          workTypes: [],
          workers: [],
          workstations: [],
          version: {
            logical: "1",
            physical: "2026-05-18T14:41:00Z",
          },
        },
      }),
    );

    await activateImportedFactoryAsNewNamedForSession(
      {
        name: "Imported Factory",
        workTypes: [],
        workers: [],
        workstations: [],
      },
      { fetch: fetchMock },
    );

    expect(openFactorySession).toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({ method: "PUT" }),
    );
  });
});
