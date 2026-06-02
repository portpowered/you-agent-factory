import { activateImportedFactoryForSession } from "./import-activation";

describe("session factory import activation create-new-named new names", () => {
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
            workTypes: [
              { name: "story", states: [{ name: "new", type: "INITIAL" }] },
            ],
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
          workTypes: [
            { name: "story", states: [{ name: "new", type: "INITIAL" }] },
          ],
          workers: [],
          workstations: [],
        },
        {
          choice: "create_new_named",
          createFactoryName: "Imported Factory Name-2",
          existingFactoryNames: [
            "Session Current Name",
            "Imported Factory Name",
          ],
          fetch: fetchMock,
        },
      ),
    ).resolves.toEqual({
      name: "Imported Factory Name-2",
      workTypes: [
        { name: "story", states: [{ name: "new", type: "INITIAL" }] },
      ],
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
            workTypes: [
              { name: "story", states: [{ name: "new", type: "INITIAL" }] },
            ],
            workers: [],
            workstations: [],
          },
        }),
      }),
    );
  });
});

describe("session factory import activation create-new-named listed existing name", () => {
  it("includes version when create-new-named upserts a name listed in existingFactoryNames", async () => {
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
            name: "Known Existing Name",
            workTypes: [
              { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
            ],
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
        workTypes: [
          { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
        ],
        workers: [],
        workstations: [],
      },
      {
        choice: "create_new_named",
        createFactoryName: "Known Existing Name",
        existingFactoryNames: ["Known Existing Name"],
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
            name: "Known Existing Name",
            workTypes: [
              { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
            ],
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
});

describe("session factory import activation create-new-named current session name", () => {
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
            workTypes: [
              { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
            ],
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
        workTypes: [
          { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
        ],
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
            workTypes: [
              { name: "task", states: [{ name: "queued", type: "INITIAL" }] },
            ],
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
});
