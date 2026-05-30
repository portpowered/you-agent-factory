import {
  isSubmitWorkAPIError,
  SubmitWorkAPIError,
  submitWork,
} from "./api";

describe("submitWork success responses", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns extended submit work identifiers from a 201 response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          accepted: true,
          name: "Driver review",
          requestId: "request-1",
          traceId: "trace-story",
          workId: "work-driver-review",
          workTypeName: "story",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 201,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      submitWork({
        name: "Driver review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).resolves.toEqual({
      accepted: true,
      name: "Driver review",
      requestId: "request-1",
      traceId: "trace-story",
      workId: "work-driver-review",
      workTypeName: "story",
    });
  });

  it("posts to the session-scoped work route when a non-default session is selected", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          accepted: true,
          name: "Session review",
          requestId: "request-session",
          sessionId: "session-beta",
          traceId: "trace-story",
          workId: "work-session-review",
          workTypeName: "story",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 201,
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      submitWork(
        {
          name: "Session review",
          payload: "Review the beta session queue.",
          workTypeName: "story",
        },
        { sessionID: "session-beta" },
      ),
    ).resolves.toEqual({
      accepted: true,
      name: "Session review",
      requestId: "request-session",
      sessionId: "session-beta",
      traceId: "trace-story",
      workId: "work-session-review",
      workTypeName: "story",
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factory-sessions/session-beta/work",
      expect.objectContaining({
        method: "POST",
      }),
    );
  });
});

describe("submitWork error handling", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("falls back to a generic message when the server returns an unstructured error", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response("server exploded", {
          headers: {
            "Content-Type": "text/plain",
          },
          status: 500,
          statusText: "Internal Server Error",
        }),
      ),
    );

    await expect(
      submitWork({
        name: "Driver review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
  });

  it("preserves structured API errors from JSON responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "INVALID_FACTORY",
            family: "BAD_REQUEST",
            message: "Work type is invalid.",
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
    );

    await expect(
      submitWork({
        name: "Structured error review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        code: "INVALID_FACTORY",
        message: "Work type is invalid.",
        status: 400,
        statusText: "Bad Request",
      }),
    );
  });

  it("falls back to the generic message when a JSON error payload has no message", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "INTERNAL_ERROR",
            family: "INTERNAL_SERVER_ERROR",
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
    );

    await expect(
      submitWork({
        name: "Missing message review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
  });

  it("falls back to the generic message when a structured JSON error message is empty", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "INTERNAL_ERROR",
            family: "INTERNAL_SERVER_ERROR",
            message: "",
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
    );

    await expect(
      submitWork({
        name: "Empty structured message review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
  });

  it("falls back to the generic message when a JSON error payload cannot be parsed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response("{not-json", {
          headers: {
            "Content-Type": "application/json",
          },
          status: 500,
          statusText: "Internal Server Error",
        }),
      ),
    );

    await expect(
      submitWork({
        name: "Malformed JSON review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
  });

  it("preserves structured errors when the payload omits the machine code", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            family: "BAD_REQUEST",
            message: "Work type is invalid.",
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
    );

    await expect(
      submitWork({
        name: "Missing code review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        code: "INTERNAL_ERROR",
        message: "Work type is invalid.",
        status: 400,
        statusText: "Bad Request",
      }),
    );
  });

  it("falls back to the generic error when the response exposes no content-type header", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        headers: {
          get: vi.fn().mockReturnValue(null),
        },
        text: vi.fn().mockResolvedValue(""),
        status: 502,
        statusText: "Bad Gateway",
      } as unknown as Response),
    );

    await expect(
      submitWork({
        name: "Missing content type review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 502,
        statusText: "Bad Gateway",
      }),
    );
  });

  it("identifies SubmitWorkAPIError instances for widget error handling", () => {
    const apiError = new SubmitWorkAPIError({
      message: "Work type is invalid.",
      status: 400,
      statusText: "Bad Request",
    });
    expect(isSubmitWorkAPIError(apiError)).toBe(true);
    expect(isSubmitWorkAPIError(new Error("other"))).toBe(false);
  });

  it("maps unknown structured error codes to INTERNAL_ERROR", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            code: "UNKNOWN_CODE",
            family: "BAD_REQUEST",
            message: "Work type is invalid.",
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
    );

    await expect(
      submitWork({
        name: "Unknown code review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        code: "INTERNAL_ERROR",
        message: "Work type is invalid.",
        status: 400,
        statusText: "Bad Request",
      }),
    );
  });

  it("falls back to the generic message when the error response body is empty", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response("", {
          headers: {
            "Content-Type": "application/json",
          },
          status: 503,
          statusText: "Service Unavailable",
        }),
      ),
    );

    await expect(
      submitWork({
        name: "Empty body review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 503,
        statusText: "Service Unavailable",
      }),
    );
  });
});
