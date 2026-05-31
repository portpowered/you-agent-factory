import { SessionFactoryAPIError } from "./errors";
import {
  activateImportedFactoryForSession,
  discoverSessionNamedFactoryNames,
} from "./import-activation";
import { defaultSessionFactoryVersion } from "./import-activation.test-helpers";

describe("session factory import activation errors — preserves non-timestamp version physical values when upserting the current factory name", () => {
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

describe("session factory import activation errors — maps current-factory GET failures during replace activation", () => {
  it("maps current-factory GET failures during replace activation", async () => {
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
        { fetch: fetchMock },
      ),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
    });
  });
});

describe("session factory import activation errors — maps unknown API error codes from replace import activation to INTERNAL_ERROR", () => {
  it("maps unknown API error codes from replace import activation to INTERNAL_ERROR", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "Session Current Name",
            workTypes: [],
            workers: [],
            workstations: [],
            version: defaultSessionFactoryVersion,
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
            code: "WEIRD_API_CODE",
            message: "Activation rejected.",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 500,
            statusText: "Internal Server Error",
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
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      responseBody: {
        code: "WEIRD_API_CODE",
        message: "Activation rejected.",
      },
      status: 500,
      statusText: "Internal Server Error",
    });
  });
});
