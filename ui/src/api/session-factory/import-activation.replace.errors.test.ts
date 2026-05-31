import {
  activateImportedFactoryForSession,
  discoverSessionNamedFactoryNames,
} from "./import-activation";
import { SessionFactoryAPIError } from "./errors";

const defaultSessionFactoryVersion = {
  logical: "9",
  physical: "2026-05-18T14:25:00Z",
} as const;

describe("session factory import activation replace errors", () => {
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
