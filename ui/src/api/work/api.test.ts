import { submitWork } from "./api";

describe("submitWork routing", () => {
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
      "/factory-sessions/~default/work",
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
      "/factory-sessions/~default/work",
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
});
