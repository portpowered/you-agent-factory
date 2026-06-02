import { SessionFactoryAPIError } from "./errors";
import { activateImportedFactoryForSession } from "./import-activation";

const defaultSessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
} as const;

describe("session factory import activation replace GET errors", () => {
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

describe("session factory import activation replace PUT conflict errors", () => {
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

describe("session factory import activation replace PUT internal errors", () => {
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
