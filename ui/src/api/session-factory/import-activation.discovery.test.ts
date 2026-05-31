import { SessionFactoryAPIError } from "./errors";
import {
  activateImportedFactoryForSession,
  discoverSessionNamedFactoryNames,
} from "./import-activation";
import { defaultSessionFactoryVersion } from "./import-activation.test-helpers";

describe("session factory import activation discovery — discovers sorted named factories for the active session folder", () => {
  it("discovers sorted named factories for the active session folder", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessions: [
              {
                factoryDir: "/workspace/project/beta",
                folderPath: "/workspace/project",
                id: "session-beta",
                isDefault: false,
                project: "beta",
                target: {
                  kind: "named",
                  name: "beta",
                },
              },
            ],
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            session: {
              factoryDir: "/workspace/project/beta",
              folderPath: "/workspace/project",
              id: "session-beta",
              isDefault: false,
              project: "beta",
              target: {
                kind: "named",
                name: "beta",
              },
            },
            targets: [
              {
                factoryDir: "/workspace/project/beta",
                folderPath: "/workspace/project",
                label: "Beta",
                project: "beta",
                ref: {
                  kind: "named",
                  name: "beta",
                },
              },
              {
                factoryDir: "/workspace/project/alpha",
                folderPath: "/workspace/project",
                label: "Alpha",
                project: "alpha",
                ref: {
                  kind: "named",
                  name: "alpha",
                },
              },
            ],
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
          },
        ),
      );

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchMock,
        sessionID: "session-beta",
      }),
    ).resolves.toEqual(["alpha", "beta"]);

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/factory-sessions", {
      method: "GET",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          folderPath: "/workspace/project",
          validateOnly: true,
        }),
      }),
    );
  });
});

describe("session factory import activation discovery — returns no discovered names when the session id is unknown", () => {
  it("returns no discovered names when the session id is unknown", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          sessions: [
            {
              factoryDir: "/workspace/project/beta",
              folderPath: "/workspace/project",
              id: "session-beta",
              isDefault: false,
              project: "beta",
              target: {
                kind: "named",
                name: "beta",
              },
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchMock,
        sessionID: "session-missing",
      }),
    ).resolves.toEqual([]);
  });
});

describe("session factory import activation discovery — returns no discovered names when the session folder path is missing", () => {
  it("returns no discovered names when the session folder path is missing", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          sessions: [
            {
              factoryDir: "/workspace/project/beta",
              id: "session-beta",
              isDefault: false,
              project: "beta",
              target: {
                kind: "named",
                name: "beta",
              },
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    await expect(
      discoverSessionNamedFactoryNames({
        fetch: fetchMock,
        sessionID: "session-beta",
      }),
    ).resolves.toEqual([]);
  });
});
