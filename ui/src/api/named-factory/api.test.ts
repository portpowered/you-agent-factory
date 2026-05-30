import { sessionFactoryOperatorErrorMessages } from "../session-factory";
import {
  activateImportedFactoryForSession,
  createFactory,
  getCurrentFactory,
  NamedFactoryAPIError,
} from "./api";

describe("factory API", () => {
  it("posts the direct canonical factory payload to /factories and returns the canonical response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          name: "Dropped Factory",
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
      "/factories",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Dropped Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      }),
    );
  });

  it("maps structured activation failures into a typed API error", async () => {
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
      new NamedFactoryAPIError(sessionFactoryOperatorErrorMessages.FACTORY_NOT_IDLE, {
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
      new NamedFactoryAPIError(sessionFactoryOperatorErrorMessages.STALE_FACTORY_VERSION, {
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

  it("activates create-new-named imports through UPSERT_NAMED_AND_ACTIVATE without version for new names", async () => {
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
            name: "Imported Factory Name-2",
            workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
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
      activateImportedFactoryForSession(
        {
          name: "Imported Factory Name",
          workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
          workers: [],
          workstations: [],
        },
        {
          choice: "create_new_named",
          createFactoryName: "Imported Factory Name-2",
          existingFactoryNames: ["Session Current Name", "Imported Factory Name"],
          fetch: fetchMock,
        },
      ),
    ).resolves.toEqual({
      name: "Imported Factory Name-2",
      workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
      workers: [],
      workstations: [],
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Imported Factory Name-2",
            workTypes: [{ name: "story", states: [{ name: "new", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
          },
        }),
      }),
    );
  });

  it("includes version when create-new-named upserts the current session factory name", async () => {
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
            workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
            version: {
              logical: "10",
              physical: "2026-05-18T14:25:00.001Z",
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

    await activateImportedFactoryForSession(
      {
        name: "Imported Payload",
        workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
        workers: [],
        workstations: [],
      },
      {
        choice: "create_new_named",
        createFactoryName: "Session Current Name",
        existingFactoryNames: ["Session Current Name"],
        fetch: fetchMock,
      },
    );

    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "UPSERT_NAMED_AND_ACTIVATE",
          factory: {
            name: "Session Current Name",
            workTypes: [{ name: "task", states: [{ name: "queued", type: "INITIAL" }] }],
            workers: [],
            workstations: [],
            version: {
              logical: "10",
              physical: "2026-05-18T14:25:00.001Z",
            },
          },
        }),
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
});
