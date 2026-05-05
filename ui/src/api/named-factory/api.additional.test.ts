import { createFactory, getCurrentFactory, NamedFactoryAPIError } from "./api";

const canonicalFactory = {
  name: "Current Factory",
  workTypes: [],
  workers: [],
  workstations: [],
} as const;

describe("named factory API error handling", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("fails fast when activation fetch is unavailable", async () => {
    await expect(
      createFactory(canonicalFactory, {
        fetch: true as unknown as typeof fetch,
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Named factory activation is unavailable in this environment.", {
        code: "NETWORK_ERROR",
      }),
    );
  });

  it("rejects activation responses that are not shaped like a factory object", async () => {
    await expect(
      createFactory(canonicalFactory, {
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
      new NamedFactoryAPIError("The factory activation API returned an invalid response.", {
        code: "INTERNAL_ERROR",
        responseBody: "not-a-factory",
        status: 200,
        statusText: "OK",
      }),
    );
  });

  it("wraps activation network failures in a typed error", async () => {
    const networkError = new Error("socket closed");

    await expect(
      createFactory(canonicalFactory, {
        fetch: vi.fn().mockRejectedValue(networkError),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("The dashboard could not reach the factory activation API.", {
        code: "NETWORK_ERROR",
        responseBody: networkError,
      }),
    );
  });

  it("falls back to INTERNAL_ERROR when the activation API returns an unknown error code", async () => {
    await expect(
      createFactory(canonicalFactory, {
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ code: "SOMETHING_NEW", message: "Activation failed." }), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 500,
            statusText: "Internal Server Error",
          }),
        ),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Activation failed.", {
        code: "INTERNAL_ERROR",
        responseBody: {
          code: "SOMETHING_NEW",
          message: "Activation failed.",
        },
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
  });

  it("preserves BAD_REQUEST activation failures as typed API errors", async () => {
    await expect(
      createFactory(canonicalFactory, {
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ code: "BAD_REQUEST", message: "Factory name is required." }), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 400,
            statusText: "Bad Request",
          }),
        ),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Factory name is required.", {
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

  it("falls back to the default activation error message when the error body has no string fields", async () => {
    await expect(
      createFactory(canonicalFactory, {
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
      new NamedFactoryAPIError("The factory activation API rejected the request.", {
        code: "INTERNAL_ERROR",
        responseBody: {
          code: 42,
          message: false,
        },
        status: 400,
        statusText: "Bad Request",
      }),
    );
  });

  it("rejects a current-factory response that is not shaped like a factory object", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify("not-an-object"), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 200,
            statusText: "OK",
          }),
        ),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("The current factory API returned an invalid response.", {
        code: "INTERNAL_ERROR",
        responseBody: "not-an-object",
        status: 200,
        statusText: "OK",
      }),
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
      new NamedFactoryAPIError("The current factory API rejected the request.", {
        code: "INTERNAL_ERROR",
        responseBody: null,
        status: 503,
        statusText: "Service Unavailable",
      }),
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
      new NamedFactoryAPIError("A named factory with this name already exists.", {
        code: "FACTORY_ALREADY_EXISTS",
        responseBody: {
          code: "FACTORY_ALREADY_EXISTS",
          message: "A named factory with this name already exists.",
        },
        status: 409,
        statusText: "Conflict",
      }),
    );
  });

  it("fails fast when current-factory fetch is unavailable", async () => {
    await expect(
      getCurrentFactory({
        fetch: true as unknown as typeof fetch,
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("Current factory export is unavailable in this environment.", {
        code: "NETWORK_ERROR",
      }),
    );
  });

  it("wraps current-factory network failures in a typed error", async () => {
    const networkError = new Error("socket closed");

    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockRejectedValue(networkError),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("The dashboard could not reach the current factory API.", {
        code: "NETWORK_ERROR",
        responseBody: networkError,
      }),
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
      new NamedFactoryAPIError("The current factory API rejected the request.", {
        code: "INTERNAL_ERROR",
        responseBody: "temporarily unavailable",
        status: 503,
        statusText: "Service Unavailable",
      }),
    );
  });

  it("falls back to the default current-factory error message when the error body has no string fields", async () => {
    await expect(
      getCurrentFactory({
        fetch: vi.fn().mockResolvedValue(
          new Response(JSON.stringify({ code: 42, message: false }), {
            headers: {
              "Content-Type": "application/json",
            },
            status: 503,
            statusText: "Service Unavailable",
          }),
        ),
      }),
    ).rejects.toEqual(
      new NamedFactoryAPIError("The current factory API rejected the request.", {
        code: "INTERNAL_ERROR",
        responseBody: {
          code: 42,
          message: false,
        },
        status: 503,
        statusText: "Service Unavailable",
      }),
    );
  });
});
