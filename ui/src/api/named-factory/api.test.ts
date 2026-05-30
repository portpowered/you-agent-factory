import {
  activateImportedFactoryAsNewNamedForSession,
  activateImportedFactoryForSession,
  createFactory,
  discoverSessionNamedFactoryNames,
  getCurrentFactory,
  NamedFactoryAPIError,
} from "./api";

describe("factory API", () => {
  it("upserts a named factory through PUT /factory-sessions/~default/factory with UPSERT_NAMED_AND_ACTIVATE", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Dropped Factory",
          workTypes: [],
          workers: [],
          workstations: [],
          version: {
            logical: "1",
            physical: "2026-05-18T14:41:00Z",
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

    await expect(
      createFactory({
        name: "Dropped Factory",
        workTypes: [],
        workers: [],
        workstations: [],
      }, { fetch: fetchMock }),
    ).resolves.toEqual({
      name: "Dropped Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Dropped Factory",
            workTypes: [],
            workers: [],
            workstations: [],
          },
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      }),
    );
  });

  it("maps structured upsert failures into a typed API error", async () => {
    await expect(
      createFactory(
        {
          name: "Dropped Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "FACTORY_NOT_IDLE",
                message: "Current factory runtime must be idle before activation.",
              }),
              {
                headers: {
                  "Content-Type": "application/json",
                },
                status: 409,
                statusText: "Conflict",
              },
            ),
          ),
        },
      ),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Current factory runtime must be idle before activation.", {
        code: "FACTORY_NOT_IDLE",
        status: 409,
        statusText: "Conflict",
        responseBody: {
          code: "FACTORY_NOT_IDLE",
          message: "Current factory runtime must be idle before activation.",
        },
      }),
    );
  });

  it("reads the current factory as a direct canonical factory payload", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Current Factory",
          workTypes: [],
          workers: [],
          workstations: [],
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
      getCurrentFactory({
        fetch: fetchMock,
      }),
    ).resolves.toEqual({
      name: "Current Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    expect(fetchMock).toHaveBeenCalledWith("/factory-sessions/~default/factory", {
      method: "GET",
    });
  });

  it("reads the current factory through the session-scoped route for non-default sessions", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Session Factory",
          workTypes: [],
          workers: [],
          workstations: [],
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
      getCurrentFactory({
        fetch: fetchMock,
        sessionID: "session-2",
      }),
    ).resolves.toEqual({
      name: "Session Factory",
      workTypes: [],
      workers: [],
      workstations: [],
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-2/factory",
      {
        method: "GET",
      },
    );
  });

  it("activates an imported factory through PUT /factory-sessions/~default/factory with version metadata", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "Session Current Name",
            workTypes: [],
            workers: [],
            workstations: [],
            version: {
              logical: "9",
              physical: "2026-05-18T14:25:00Z",
            },
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
            name: "Session Current Name",
            workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
            version: {
              logical: "10",
              physical: "2026-05-18T14:40:00Z",
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

    await expect(
      activateImportedFactoryForSession(
        {
          name: "Imported Factory Name",
          workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
          workers: [],
          workstations: [],
        },
        { fetch: fetchMock },
      ),
    ).resolves.toEqual({
      name: "Session Current Name",
      workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
      workers: [],
      workstations: [],
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(1, "/factory-sessions/~default/factory", {
      method: "GET",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "Session Current Name",
            workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
            version: {
              logical: "10",
              physical: "2026-05-18T14:25:00.001Z",
            },
          },
        }),
        headers: {
          "content-type": "application/json",
        },
        method: "PUT",
      }),
    );
    expect(fetchMock).not.toHaveBeenCalledWith(
      "/factories",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("activates an imported factory through the session-scoped PUT route for non-default sessions", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "Scoped Factory",
            workTypes: [],
            workers: [],
            workstations: [],
            version: {
              logical: "3",
              physical: "2026-05-18T14:24:00Z",
            },
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
            name: "Scoped Factory",
            workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
            version: {
              logical: "4",
              physical: "2026-05-18T14:41:00Z",
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

    await expect(
      activateImportedFactoryForSession(
        {
          name: "Imported Scoped Name",
          workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
          workers: [],
          workstations: [],
        },
        {
          fetch: fetchMock,
          sessionID: "session-2",
        },
      ),
    ).resolves.toEqual({
      name: "Scoped Factory",
      workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
      workers: [],
      workstations: [],
    });

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/factory-sessions/session-2/factory", {
      method: "GET",
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/session-2/factory",
      expect.objectContaining({
        method: "PUT",
        body: expect.stringContaining('"name":"Scoped Factory"'),
      }),
    );
  });

  it("maps FACTORY_NOT_IDLE session save failures into named factory activation errors", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "Current Factory",
            workTypes: [],
            workers: [],
            workstations: [],
            version: {
              logical: "9",
              physical: "2026-05-18T14:25:00Z",
            },
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
            code: "FACTORY_NOT_IDLE",
            message: "Current factory runtime must be idle before activation.",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 409,
            statusText: "Conflict",
          },
        ),
      );

    await expect(
      activateImportedFactoryForSession(
        {
          name: "Imported Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        },
        { fetch: fetchMock },
      ),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Current factory runtime must be idle before activation.", {
        code: "FACTORY_NOT_IDLE",
        status: 409,
        statusText: "Conflict",
        responseBody: {
          code: "FACTORY_NOT_IDLE",
          message: "Current factory runtime must be idle before activation.",
        },
      }),
    );
  });

  it("maps STALE_FACTORY_VERSION session save failures into named factory activation errors", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "Current Factory",
            workTypes: [],
            workers: [],
            workstations: [],
            version: {
              logical: "9",
              physical: "2026-05-18T14:25:00Z",
            },
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
            code: "STALE_FACTORY_VERSION",
            message: "The editable definition is stale.",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 409,
            statusText: "Conflict",
          },
        ),
      );

    await expect(
      activateImportedFactoryForSession(
        {
          name: "Imported Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        },
        { fetch: fetchMock },
      ),
    ).rejects.toEqual(
      new NamedFactoryAPIError("The editable definition is stale.", {
        code: "STALE_FACTORY_VERSION",
        status: 409,
        statusText: "Conflict",
        responseBody: {
          code: "STALE_FACTORY_VERSION",
          message: "The editable definition is stale.",
        },
      }),
    );
  });

  it("rejects retired named-factory wrapper responses from the current factory endpoint", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              factory: {
                workTypes: [],
                workers: [],
                workstations: [],
              },
              name: "Current Factory",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 200,
              statusText: "OK",
            },
          ),
        ),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("The current factory API returned an invalid response.", {
        code: "INTERNAL_ERROR",
        responseBody: {
          factory: {
            workTypes: [],
            workers: [],
            workstations: [],
          },
          name: "Current Factory",
        },
        status: 200,
        statusText: "OK",
      }),
    );
  });

  it("activates an imported factory as a new named factory with UPSERT_NAMED_AND_ACTIVATE", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessions: [
              {
                factoryDir: "/tmp/factories",
                folderPath: "/tmp/factories",
                id: "~default",
                isDefault: true,
                project: "default",
                target: { kind: "default" },
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
              {
                factoryDir: "/tmp/factories/Dropped Factory",
                folderPath: "/tmp/factories",
                label: "Dropped Factory",
                project: "Dropped Factory",
                ref: { kind: "named", name: "Dropped Factory" },
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
            name: "Dropped Factory-2",
            workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
            version: {
              logical: "1",
              physical: "2026-05-18T14:41:00Z",
            },
          }),
          {
            headers: { "Content-Type": "application/json" },
            status: 200,
          },
        ),
      );

    await expect(
      activateImportedFactoryAsNewNamedForSession(
        {
          name: "Dropped Factory",
          workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
          workers: [],
          workstations: [],
        },
        { fetch: fetchMock },
      ),
    ).resolves.toEqual({
      name: "Dropped Factory-2",
      workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
      workers: [],
      workstations: [],
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Dropped Factory-2",
            workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
          },
        }),
      }),
    );
  });

  it("discovers named factory names from validate-only session targets", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            sessions: [
              {
                factoryDir: "/tmp/factories",
                folderPath: "/tmp/factories",
                id: "session-2",
                isDefault: false,
                project: "review",
                target: { kind: "named", name: "alpha" },
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
              {
                factoryDir: "/tmp/factories/alpha",
                folderPath: "/tmp/factories",
                label: "Alpha",
                project: "alpha",
                ref: { kind: "named", name: "alpha" },
              },
              {
                factoryDir: "/tmp/factories/beta",
                folderPath: "/tmp/factories",
                label: "Beta",
                project: "beta",
                ref: { kind: "named", name: "beta" },
              },
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
        sessionID: "session-2",
      }),
    ).resolves.toEqual(["alpha", "beta"]);
  });
});
