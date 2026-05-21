import {
  extractAPIErrorPayload,
  isAPIRecord,
  readAPIResponseBody,
} from "./transport";

describe("API transport helper", () => {
  it("returns null for empty response bodies", async () => {
    const body = await readAPIResponseBody(
      new Response(null, {
        status: 204,
        statusText: "No Content",
      }),
    );

    expect(body).toBeNull();
  });

  it("parses valid JSON response bodies", async () => {
    const body = await readAPIResponseBody(
      new Response(JSON.stringify({ code: "BAD_REQUEST" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 400,
        statusText: "Bad Request",
      }),
    );

    expect(body).toEqual({ code: "BAD_REQUEST" });
  });

  it("falls back to the raw response text for non-JSON bodies", async () => {
    const body = await readAPIResponseBody(
      new Response("gateway timeout", {
        status: 504,
        statusText: "Gateway Timeout",
      }),
    );

    expect(body).toBe("gateway timeout");
  });

  it("treats only plain objects as API records", () => {
    expect(isAPIRecord({ code: "BAD_REQUEST" })).toBe(true);
    expect(isAPIRecord(["BAD_REQUEST"])).toBe(false);
    expect(isAPIRecord(null)).toBe(false);
  });

  it("extracts API error fields only when code or message is present", () => {
    expect(
      extractAPIErrorPayload({
        code: "BAD_REQUEST",
        message: "The request was invalid.",
      }),
    ).toEqual({
      code: "BAD_REQUEST",
      message: "The request was invalid.",
      targets: undefined,
    });

    expect(
      extractAPIErrorPayload({
        details: "missing typed fields",
        targets: [{ kind: "field", path: "factory.name" }],
      }),
    ).toBeNull();
  });

  it("preserves structured targets when the caller provides a target validator", () => {
    const payload = extractAPIErrorPayload(
      {
        code: "BAD_REQUEST",
        message: "The request was invalid.",
        targets: [
          {
            kind: "field",
            path: "factory.name",
          },
        ],
      },
      {
        isTarget: (value): value is { kind: string; path: string } =>
          isAPIRecord(value) &&
          typeof value.kind === "string" &&
          typeof value.path === "string",
      },
    );

    expect(payload).toEqual({
      code: "BAD_REQUEST",
      message: "The request was invalid.",
      targets: [
        {
          kind: "field",
          path: "factory.name",
        },
      ],
    });
  });
});
