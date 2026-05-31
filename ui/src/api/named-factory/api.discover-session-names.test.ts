import { afterEach, describe, expect, it, vi } from "bun:test";

import { discoverSessionNamedFactoryNames } from "./api";

describe("discoverSessionNamedFactoryNames", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns an empty list when the session has no folder path", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          sessions: [
            { id: "~default", folderPath: null },
            { id: "session-review", folderPath: "" },
          ],
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 200,
        },
      ),
    );

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchMock,
        sessionID: "session-review",
      }),
    ).resolves.toEqual([]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("opens the session folder and returns sorted named factory targets", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessions: [
              {
                id: "session-review",
                folderPath: "/tmp/factory-session",
              },
            ],
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 200,
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            targets: [
              { ref: { kind: "named", name: "gamma" } },
              { ref: { kind: "named", name: "alpha" } },
              { ref: { kind: "bundled", path: "ignored" } },
            ],
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 200,
          },
        ),
      );

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchMock,
        sessionID: "session-review",
      }),
    ).resolves.toEqual(["alpha", "gamma"]);

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/factory-sessions", {
      method: "GET",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          folderPath: "/tmp/factory-session",
          validateOnly: true,
        }),
      }),
    );
  });

  it("discovers names for the default session when sessionID is omitted", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessions: [
              {
                id: "~default",
                folderPath: "/tmp/default-session",
              },
            ],
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 200,
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            targets: [{ ref: { kind: "named", name: "alpha" } }],
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 200,
          },
        ),
      );

    await expect(
      discoverSessionNamedFactoryNames({ fetch: fetchMock }),
    ).resolves.toEqual(["alpha"]);
  });
});
