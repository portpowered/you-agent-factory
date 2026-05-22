import { SubmitWorkAPIError, submitWork } from "./api";

describe("submitWork", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts to the explicit default-session work route and returns the accepted trace id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ traceId: "trace-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      submitWork({
        name: "Driver review",
        payload: "Review the runtime failure.",
        workTypeName: "story",
      }),
    ).resolves.toEqual({ traceId: "trace-story" });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factories/~default/work",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Driver review",
          payload: "Review the runtime failure.",
          workTypeName: "story",
        }),
        method: "POST",
      }),
    );
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

  it("posts an explicit empty payload through the default-session scoped route without dropping the submit-work contract field", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ traceId: "trace-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      submitWork({
        name: "Empty payload review",
        payload: "",
        workTypeName: "story",
      }),
    ).resolves.toEqual({ traceId: "trace-story" });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factories/~default/work",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Empty payload review",
          payload: "",
          workTypeName: "story",
        }),
        method: "POST",
      }),
    );
  });

  it("posts to the session-scoped work route when a non-default session is selected", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ traceId: "trace-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
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
    ).resolves.toEqual({ traceId: "trace-story" });
    expect(fetchMock).toHaveBeenCalledWith(
      "/factories/session-beta/work",
      expect.objectContaining({
        method: "POST",
      }),
    );
  });
});
