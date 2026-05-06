import { SubmitWorkAPIError, submitWork } from "./api";

describe("submitWork error handling", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
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
        payload: "Review the runtime failure.",
        work_type_name: "story",
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
        new Response(JSON.stringify({ code: "INTERNAL_ERROR", family: "INTERNAL_SERVER_ERROR" }), {
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
        payload: "Review the runtime failure.",
        work_type_name: "story",
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
        json: vi.fn(),
        status: 502,
        statusText: "Bad Gateway",
      } as unknown as Response),
    );

    await expect(
      submitWork({
        payload: "Review the runtime failure.",
        work_type_name: "story",
      }),
    ).rejects.toEqual(
      new SubmitWorkAPIError({
        message: "Dashboard submission failed. Try again in a moment.",
        status: 502,
        statusText: "Bad Gateway",
      }),
    );
  });
});
