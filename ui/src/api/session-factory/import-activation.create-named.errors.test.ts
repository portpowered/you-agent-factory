import {
  activateImportedFactoryForSession,
  discoverSessionNamedFactoryNames,
} from "./import-activation";
import { SessionFactoryAPIError } from "./errors";

const defaultSessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
} as const;

describe("session factory import activation create-new-named errors", () => {
  it("requires a factory name for create-new-named activation", async () => {
    await expect(
      activateImportedFactoryForSession(
        {
          name: "Imported Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        },
        {
          choice: "create_new_named",
          createFactoryName: "   ",
        },
      ),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY_NAME",
    });
  });

  it("maps current-factory GET failures during create-new-named activation", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
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
        {
          choice: "create_new_named",
          createFactoryName: "Imported Factory-2",
          fetch: fetchMock,
        },
      ),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
    });
  });

  it("maps session-factory PUT failures during create-new-named activation", async () => {
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
            code: "INVALID_FACTORY",
            message: "Factory definition rejected.",
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

    await expect(
      activateImportedFactoryForSession(
        {
          name: "Imported Factory",
          workTypes: [],
          workers: [],
          workstations: [],
        },
        {
          choice: "create_new_named",
          createFactoryName: "Imported Factory-2",
          fetch: fetchMock,
        },
      ),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY",
    });
  });

  it("preserves non-timestamp version physical values when upserting the current factory name", async () => {
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
              physical: "legacy-physical",
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
            workTypes: [],
            workers: [],
            workstations: [],
            version: {
              logical: "10",
              physical: "legacy-physical",
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
        workTypes: [],
        workers: [],
        workstations: [],
      },
      {
        choice: "create_new_named",
        createFactoryName: "Session Current Name",
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
            workTypes: [],
            workers: [],
            workstations: [],
            version: {
              logical: "10",
              physical: "legacy-physical",
            },
          },
        }),
      }),
    );
  });
});
