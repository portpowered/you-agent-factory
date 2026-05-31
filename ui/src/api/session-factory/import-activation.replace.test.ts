import { activateImportedFactoryForSession } from "./import-activation";

describe("session factory import activation replace-current default session", () => {
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
});

describe("session factory import activation replace-current scoped session", () => {
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
        body: expect.stringContaining('"mode":"REPLACE_CURRENT"'),
      }),
    );
  });
});
