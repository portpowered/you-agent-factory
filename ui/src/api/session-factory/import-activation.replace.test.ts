import { SessionFactoryAPIError } from "./errors";
import {
  activateImportedFactoryForSession,
  discoverSessionNamedFactoryNames,
} from "./import-activation";
import { defaultSessionFactoryVersion } from "./import-activation.test-helpers";

describe("session factory import activation replace — activates an imported factory through PUT /factory-sessions/~default/factory with version metadata", () => {
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
            workTypes: [
              { name: "story", states: [{ name: "new", type: "INITIAL" }] },
            ],
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
          workTypes: [
            { name: "story", states: [{ name: "new", type: "INITIAL" }] },
          ],
          workers: [],
          workstations: [],
        },
        { fetch: fetchMock },
      ),
    ).resolves.toEqual({
      name: "Session Current Name",
      workTypes: [
        { name: "story", states: [{ name: "new", type: "INITIAL" }] },
      ],
      workers: [],
      workstations: [],
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/factory-sessions/~default/factory",
      {
        method: "GET",
      },
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      "/factory-sessions/~default/factory",
      expect.objectContaining({
        body: JSON.stringify({
          mode: "REPLACE_CURRENT",
          factory: {
            name: "Session Current Name",
            workTypes: [
              { name: "story", states: [{ name: "new", type: "INITIAL" }] },
            ],
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

describe("session factory import activation replace — activates an imported factory through the session-scoped PUT route for non-default sessions", () => {
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
            workTypes: [
              { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
            ],
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
          workTypes: [
            { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
          ],
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
      workTypes: [
        { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
      ],
      workers: [],
      workstations: [],
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/factory-sessions/session-2/factory",
      {
        method: "GET",
      },
    );
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

describe("session factory import activation replace — maps FACTORY_NOT_IDLE session save failures into session factory import activation errors", () => {
  it("maps FACTORY_NOT_IDLE session save failures into session factory import activation errors", async () => {
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
      new SessionFactoryAPIError(
        "The current factory runtime is still active. Wait until it becomes idle before saving or switching factories.",
        {
          code: "FACTORY_NOT_IDLE",
          status: 409,
          statusText: "Conflict",
          responseBody: {
            code: "FACTORY_NOT_IDLE",
            message: "Current factory runtime must be idle before activation.",
          },
        },
      ),
    );
  });
});

describe("session factory import activation replace — maps STALE_FACTORY_VERSION session save failures into session factory import activation errors", () => {
  it("maps STALE_FACTORY_VERSION session save failures into session factory import activation errors", async () => {
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
      new SessionFactoryAPIError(
        "Current factory definition is stale. Refresh the dashboard before saving or importing again.",
        {
          code: "STALE_FACTORY_VERSION",
          status: 409,
          statusText: "Conflict",
          responseBody: {
            code: "STALE_FACTORY_VERSION",
            message: "The editable definition is stale.",
          },
        },
      ),
    );
  });
});
