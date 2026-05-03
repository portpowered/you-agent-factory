import {
  SubmitWorkAPIError,
  submitWork,
} from "./api";

describe("submitWork", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("posts to /work and returns the accepted trace id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ trace_id: "trace-story" }), {
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
        work_type_name: "story",
      }),
    ).resolves.toEqual({ trace_id: "trace-story" });
    expect(fetchMock).toHaveBeenCalledWith(
      "/work",
      expect.objectContaining({
        body: JSON.stringify({
          name: "Driver review",
          payload: "Review the runtime failure.",
          work_type_name: "story",
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
        payload: "Review the runtime failure.",
        work_type_name: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 500,
        statusText: "Internal Server Error",
      }),
    );
  });
});

