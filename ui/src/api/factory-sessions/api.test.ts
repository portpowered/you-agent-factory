import {
  closeFactorySession,
  FactorySessionsAPIError,
  listFactorySessions,
  openFactorySession,
} from "./api";

describe("factory sessions API", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists live factory sessions from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          sessions: [
            {
              factoryDir: "/workspace/project/alpha",
              folderPath: "/workspace/project",
              id: "~default",
              isDefault: true,
              project: "alpha",
              target: {
                kind: "default",
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
    vi.stubGlobal("fetch", fetchMock);

    await expect(listFactorySessions()).resolves.toEqual([
      {
        factoryDir: "/workspace/project/alpha",
        folderPath: "/workspace/project",
        id: "~default",
        isDefault: true,
        project: "alpha",
        target: {
          kind: "default",
        },
      },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("posts folder and target selection when opening a factory session", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
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
              ref: {
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
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
        target: {
          kind: "named",
          name: "beta",
        },
      }),
    ).resolves.toEqual({
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
          ref: {
            kind: "named",
            name: "beta",
          },
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/project",
          target: {
            kind: "named",
            name: "beta",
          },
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("posts validateOnly when checking a folder before launch", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          targets: [
            {
              factoryDir: "/workspace/project",
              folderPath: "/workspace/project",
              label: "default",
              project: "project",
              ref: {
                kind: "default",
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
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
        validateOnly: true,
      }),
    ).resolves.toEqual({
      targets: [
        {
          factoryDir: "/workspace/project",
          folderPath: "/workspace/project",
          label: "default",
          project: "project",
          ref: {
            kind: "default",
          },
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/project",
          validateOnly: true,
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("posts initNewFactory when creating a factory from an empty folder", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          session: {
            factoryDir: "/workspace/new",
            folderPath: "/workspace/new",
            id: "session-new",
            isDefault: false,
            project: "new",
            target: {
              kind: "default",
            },
          },
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/new",
        initNewFactory: true,
      }),
    ).resolves.toEqual({
      session: {
        factoryDir: "/workspace/new",
        folderPath: "/workspace/new",
        id: "session-new",
        isDefault: false,
        project: "new",
        target: {
          kind: "default",
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/new",
          initNewFactory: true,
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("accepts validateOnly initsNewFactory discovery responses", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          folderPath: "/workspace/empty",
          initsNewFactory: true,
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/empty",
        validateOnly: true,
      }),
    ).resolves.toEqual({
      folderPath: "/workspace/empty",
      initsNewFactory: true,
    });
  });

  it("maps validation failures into typed API errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "BAD_REQUEST",
          message: "factory session folder is required",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "",
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError("factory session folder is required", {
        code: "BAD_REQUEST",
        responseBody: {
          code: "BAD_REQUEST",
          message: "factory session folder is required",
        },
        status: 400,
        statusText: "Bad Request",
      }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "",
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("preserves structured validation targets on typed API errors", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "BAD_REQUEST",
          message: "folder validation failed",
          targets: [
            {
              kind: "factory-session-validation",
              id: "missing",
              field: "folderPath",
            },
          ],
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/missing",
        validateOnly: true,
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError("folder validation failed", {
        code: "BAD_REQUEST",
        responseBody: {
          code: "BAD_REQUEST",
          message: "folder validation failed",
          targets: [
            {
              kind: "factory-session-validation",
              id: "missing",
              field: "folderPath",
            },
          ],
        },
        status: 400,
        statusText: "Bad Request",
        targets: [
          {
            kind: "factory-session-validation",
            id: "missing",
            field: "folderPath",
          },
        ],
      }),
    );
  });

  it("keeps the list fallback error for empty error bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(null, {
        status: 503,
        statusText: "Service Unavailable",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(listFactorySessions()).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          responseBody: null,
          status: 503,
          statusText: "Service Unavailable",
        },
      ),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({ method: "GET" }),
    );
  });

  it("keeps the open fallback error for malformed JSON bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("{", {
        headers: {
          "Content-Type": "application/json",
        },
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      openFactorySession({
        folderPath: "/workspace/project",
      }),
    ).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          responseBody: "{",
          status: 500,
          statusText: "Internal Server Error",
        },
      ),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions",
      expect.objectContaining({
        body: JSON.stringify({
          folderPath: "/workspace/project",
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("keeps the close fallback error for plain-text error bodies", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("session close failed upstream", {
        status: 502,
        statusText: "Bad Gateway",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(closeFactorySession("session-beta")).rejects.toEqual(
      new FactorySessionsAPIError(
        "The factory sessions API rejected the request.",
        {
          code: "INTERNAL_ERROR",
          responseBody: "session close failed upstream",
          status: 502,
          statusText: "Bad Gateway",
        },
      ),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta",
      expect.objectContaining({ method: "DELETE" }),
    );
  });

  it("deletes one live factory session from the typed API surface", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(null, {
        status: 204,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(closeFactorySession("session-beta")).resolves.toBeUndefined();
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta",
      expect.objectContaining({ method: "DELETE" }),
    );
  });
});
