import {
  invokeSessionFactory,
  type SessionFactoryInvocationError,
} from "./invocation";

const originalFetch = globalThis.fetch;

describe("session-factory invocation api", () => {
  afterEach(() => {
    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: originalFetch,
      writable: true,
    });
  });

  it("posts structured args to the session invocation endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          requestId: "request-1",
          status: "COMPLETED",
          traceId: "trace-1",
        }),
        {
          status: 200,
        },
      ),
    );
    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: fetchMock,
      writable: true,
    });

    const response = await invokeSessionFactory(
      {
        args: {
          input: "hello world",
          tags: ["alpha", "beta"],
        },
      },
      { sessionID: "session-beta" },
    );

    expect(response.traceId).toBe("trace-1");
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/invocations",
      {
        body: JSON.stringify({
          args: {
            input: "hello world",
            tags: ["alpha", "beta"],
          },
        }),
        headers: {
          "Content-Type": "application/json",
        },
        method: "POST",
      },
    );
  });

  it("surfaces machine-readable bad-request failures from the invocation API", async () => {
    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT",
            message: 'required invocation parameter "input" is missing',
          }),
          {
            status: 400,
            statusText: "Bad Request",
          },
        ),
      ),
      writable: true,
    });

    await expect(
      invokeSessionFactory({
        args: {},
      }),
    ).rejects.toMatchObject<Partial<SessionFactoryInvocationError>>({
      code: "INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT",
      message: 'required invocation parameter "input" is missing',
      status: 400,
      statusText: "Bad Request",
    });
  });

  it("rejects invalid success payloads from the invocation API", async () => {
    Object.defineProperty(globalThis, "fetch", {
      configurable: true,
      value: vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ traceId: "trace-1" }), {
          status: 200,
        }),
      ),
      writable: true,
    });

    await expect(
      invokeSessionFactory({
        args: {
          input: "hello",
        },
      }),
    ).rejects.toMatchObject<Partial<SessionFactoryInvocationError>>({
      code: "INTERNAL_ERROR",
      message: "The session invocation API returned an invalid response.",
    });
  });
});
