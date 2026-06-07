import { currentFactoryDefinitionAPIErrorMessages } from "../current-factory-definition/messages";
import { SessionFactoryAPIError } from "./errors";
import {
  activateImportedFactoryForSession,
  getCurrentFactory,
} from "./import-activation";

const canonicalFactory = {
  name: "Current Factory",
  workTypes: [],
  workers: [],
  workstations: [],
} as const;

describe("session factory getCurrentFactory error handling", () => {
  it("fails fast when current-factory fetch is unavailable", async () => {
    await expect(
      getCurrentFactory({
        fetch: true as unknown as typeof fetch,
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        "Current factory export is unavailable in this environment.",
        {
          code: "NETWORK_ERROR",
        },
      ),
    );
  });

  it("rejects current-factory responses that are not shaped like a factory object", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify("not-a-factory"), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
            statusText: "OK",
          }),
        ),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        currentFactoryDefinitionAPIErrorMessages.invalidResponse,
        {
          code: "INTERNAL_ERROR",
          responseBody: "not-a-factory",
          status: 200,
          statusText: "OK",
        },
      ),
    );
  });

  it("wraps current-factory network failures in a typed error", async () => {
    const networkError = new Error("socket closed");

    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockRejectedValue(networkError),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        currentFactoryDefinitionAPIErrorMessages.network,
        {
          code: "NETWORK_ERROR",
          responseBody: networkError,
        },
      ),
    );
  });

  it("falls back to INTERNAL_ERROR when the current-factory API returns an unknown error code", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "SOMETHING_NEW",
              message: "Lookup failed.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 500,
              statusText: "Internal Server Error",
            },
          ),
        ),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        currentFactoryDefinitionAPIErrorMessages.rejectedRequest,
        {
          code: "INTERNAL_ERROR",
          responseBody: {
            code: "SOMETHING_NEW",
            message: "Lookup failed.",
          },
          status: 500,
          statusText: "Internal Server Error",
        },
      ),
    );
  });

  it("preserves BAD_REQUEST current-factory failures as typed API errors", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "BAD_REQUEST",
              message: "Factory name is required.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 400,
              statusText: "Bad Request",
            },
          ),
        ),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError("Factory name is required.", {
        code: "BAD_REQUEST",
        responseBody: {
          code: "BAD_REQUEST",
          message: "Factory name is required.",
        },
        status: 400,
        statusText: "Bad Request",
      }),
    );
  });

  it("maps NOT_FOUND current-factory failures to SessionFactoryAPIError", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "NOT_FOUND",
              message: "Session factory not found.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 404,
              statusText: "Not Found",
            },
          ),
        ),
      }),
    ).rejects.toMatchObject({
      code: "NOT_FOUND",
      message: "Session factory not found.",
      status: 404,
    });
  });

  it("maps STALE_FACTORY_VERSION current-factory failures to SessionFactoryAPIError", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "STALE_FACTORY_VERSION",
              message: "The session factory version is stale.",
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
      }),
    ).rejects.toMatchObject({
      code: "STALE_FACTORY_VERSION",
      status: 409,
    });
  });

  it("maps FACTORY_NOT_IDLE current-factory failures to SessionFactoryAPIError", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "FACTORY_NOT_IDLE",
              message: "Current factory runtime must be idle before export.",
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
      }),
    ).rejects.toMatchObject({
      code: "FACTORY_NOT_IDLE",
      status: 409,
    });
  });

  it("maps INVALID_FACTORY_DEFINITION current-factory failures to INVALID_FACTORY", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Broken Factory",
              workers: [],
              workstations: [],
              workTypes: [],
              version: {
                logical: "1",
              },
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
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY",
      name: "SessionFactoryAPIError",
    });
  });

  it("falls back to the default current-factory error message when the error body has no string fields", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ code: 42, message: false }), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 400,
            statusText: "Bad Request",
          }),
        ),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        currentFactoryDefinitionAPIErrorMessages.rejectedRequest,
        {
          code: "INTERNAL_ERROR",
          responseBody: {
            code: 42,
            message: false,
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
  });

  it("maps FACTORY_NOT_IDLE session save failures into session factory activation errors", async () => {
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
      activateImportedFactoryForSession(canonicalFactory, { fetch: fetchMock }),
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

  it("surfaces the default current-factory rejection message for empty error bodies", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(null, {
            status: 503,
            statusText: "Service Unavailable",
          }),
        ),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        currentFactoryDefinitionAPIErrorMessages.rejectedRequest,
        {
          code: "INTERNAL_ERROR",
          responseBody: null,
          status: 503,
          statusText: "Service Unavailable",
        },
      ),
    );
  });

  it("preserves FACTORY_ALREADY_EXISTS errors from current-factory lookups when the API reports them", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "FACTORY_ALREADY_EXISTS",
              message: "A named factory with this name already exists.",
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
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        "A named factory with this name already exists.",
        {
          code: "FACTORY_ALREADY_EXISTS",
          responseBody: {
            code: "FACTORY_ALREADY_EXISTS",
            message: "A named factory with this name already exists.",
          },
          status: 409,
          statusText: "Conflict",
        },
      ),
    );
  });

  it("preserves raw current-factory error bodies when the response is not JSON", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response("temporarily unavailable", {
            status: 503,
            statusText: "Service Unavailable",
          }),
        ),
      }),
    ).rejects.toEqual(
      new SessionFactoryAPIError(
        currentFactoryDefinitionAPIErrorMessages.rejectedRequest,
        {
          code: "INTERNAL_ERROR",
          responseBody: "temporarily unavailable",
          status: 503,
          statusText: "Service Unavailable",
        },
      ),
    );
  });
});
