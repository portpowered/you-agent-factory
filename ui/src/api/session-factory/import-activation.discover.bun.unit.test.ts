import { describe, expect, it } from "bun:test";

import { discoverSessionNamedFactoryNames } from "./import-activation";

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status: 200,
  });
}

function queuedFetch(responses: readonly Response[]) {
  const pending = [...responses];
  const calls: Array<{
    input: Parameters<typeof globalThis.fetch>[0];
    init: Parameters<typeof globalThis.fetch>[1];
  }> = [];
  const fetchImplementation: typeof globalThis.fetch = async (input, init) => {
    calls.push({ input, init });
    const response = pending.shift();
    if (!response) {
      throw new Error("The discovery fixture received an unexpected request.");
    }
    return response;
  };
  return { calls, fetchImplementation };
}

function factorySessionSummary(
  id: string,
  folderPath: string | null,
  isDefault = false,
) {
  return {
    factoryDir: folderPath ? `${folderPath}/factory` : "/workspace/factory",
    ...(folderPath === null ? {} : { folderPath }),
    id,
    isDefault,
    project: id,
    target: isDefault
      ? { kind: "default" }
      : { kind: "named", name: id },
  };
}

function openTargetsResponse() {
  return {
    targets: [
      {
        factoryDir: "/workspace/project/gamma",
        folderPath: "/workspace/project",
        label: "Gamma",
        project: "gamma",
        ref: { kind: "named", name: "gamma" },
      },
      {
        factoryDir: "/workspace/project/alpha",
        folderPath: "/workspace/project",
        label: "Alpha",
        project: "alpha",
        ref: { kind: "named", name: "alpha" },
      },
    ],
  };
}

describe("discoverSessionNamedFactoryNames", () => {
  it("returns an empty list when the session has no folder path", async () => {
    const { fetchImplementation, calls } = queuedFetch([
      jsonResponse({
        sessions: [
          factorySessionSummary("session-review", null),
          factorySessionSummary("session-other", "/workspace/other"),
        ],
      }),
    ]);

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchImplementation,
        sessionID: "session-review",
      }),
    ).resolves.toEqual([]);
    expect(calls).toHaveLength(1);
    expect(calls[0]?.init).toMatchObject({ method: "GET" });
  });

  it("opens the session folder and returns sorted named factory targets", async () => {
    const { fetchImplementation, calls } = queuedFetch([
      jsonResponse({
        sessions: [
          factorySessionSummary("session-review", "/tmp/factory-session"),
        ],
      }),
      jsonResponse(openTargetsResponse()),
    ]);

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchImplementation,
        sessionID: "session-review",
      }),
    ).resolves.toEqual(["alpha", "gamma"]);
    expect(calls).toHaveLength(2);
    expect(calls[1]?.init).toMatchObject({
      body: JSON.stringify({
        folderPath: "/tmp/factory-session",
        validateOnly: true,
      }),
      method: "POST",
    });
  });

  it("discovers names for the default session when sessionID is omitted", async () => {
    const { fetchImplementation, calls } = queuedFetch([
      jsonResponse({
        sessions: [
          factorySessionSummary(
            "session-default",
            "/tmp/default-session",
            true,
          ),
        ],
      }),
      jsonResponse(openTargetsResponse()),
    ]);

    await expect(
      discoverSessionNamedFactoryNames({ fetch: fetchImplementation }),
    ).resolves.toEqual(["alpha", "gamma"]);
    expect(calls).toHaveLength(2);
  });
});
