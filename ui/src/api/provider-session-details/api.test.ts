import {
  getProviderSessionDetails,
  ProviderSessionDetailsAPIError,
  toProviderSessionDetailRef,
} from "./api";

describe("getProviderSessionDetails", () => {
  it("loads structured provider-session details from the typed API route", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          parse: {
            eventCount: 3,
            functionCalls: [],
            lineCount: 3,
            malformedLineCount: 0,
            parseErrors: [],
            reasoning: [],
            tokenUsage: {
              totalTokens: 42,
            },
            turns: [],
            unknownEventCount: 0,
            unknownEvents: [],
          },
          providerSession: {
            id: "sess_alpha",
            kind: "session_id",
            provider: "codex",
          },
          source: {
            relativePath: "2026/05/18/rollout-sess_alpha.jsonl",
            sizeBytes: 1234,
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
    );

    await expect(
      getProviderSessionDetails(
        {
          id: "sess_alpha",
          kind: "session_id",
          provider: "codex",
        },
        { fetch },
      ),
    ).resolves.toMatchObject({
      parse: {
        eventCount: 3,
      },
      source: {
        relativePath: "2026/05/18/rollout-sess_alpha.jsonl",
      },
    });

    expect(fetch).toHaveBeenCalledWith(
      "/provider-sessions/detail?id=sess_alpha&kind=session_id&provider=codex",
      { method: "GET" },
    );
  });

  it("surfaces not-found errors with the original API code", async () => {
    await expect(
      getProviderSessionDetails(
        {
          id: "sess_missing",
          kind: "session_id",
          provider: "codex",
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "NOT_FOUND",
                message: "Provider session sess_missing was not found.",
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
        },
      ),
    ).rejects.toMatchObject({
      code: "NOT_FOUND",
      message: "Provider session sess_missing was not found.",
      name: "ProviderSessionDetailsAPIError",
      status: 404,
      statusText: "Not Found",
    });
  });

  it("falls back to INTERNAL_ERROR for unknown provider-session API error codes while preserving the API message", async () => {
    await expect(
      getProviderSessionDetails(
        {
          id: "sess_alpha",
          kind: "session_id",
          provider: "codex",
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code: "SOMETHING_NEW",
                message: "Provider session detail loading failed.",
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
        },
      ),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "Provider session detail loading failed.",
      name: "ProviderSessionDetailsAPIError",
      responseBody: {
        code: "SOMETHING_NEW",
        message: "Provider session detail loading failed.",
      },
      status: 500,
      statusText: "Internal Server Error",
    });
  });

  it("preserves raw provider-session error bodies when the API response is not JSON", async () => {
    await expect(
      getProviderSessionDetails(
        {
          id: "sess_alpha",
          kind: "session_id",
          provider: "codex",
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response("temporarily unavailable", {
              status: 503,
              statusText: "Service Unavailable",
            }),
          ),
        },
      ),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "The provider-session detail API rejected the request.",
      name: "ProviderSessionDetailsAPIError",
      responseBody: "temporarily unavailable",
      status: 503,
      statusText: "Service Unavailable",
    });
  });

  it("rejects invalid success payloads", async () => {
    let thrown: unknown;

    try {
      await getProviderSessionDetails(
        {
          id: "sess_broken",
          kind: "session_id",
          provider: "codex",
        },
        {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                parse: {},
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
        },
      );
    } catch (error) {
      thrown = error;
    }

    expect(thrown).toBeInstanceOf(ProviderSessionDetailsAPIError);
    expect(thrown).toMatchObject({
      code: "INTERNAL_ERROR",
      message:
        "The provider-session detail API returned an invalid response.",
      status: 200,
      statusText: "OK",
    });
  });

  it("canonicalizes supported provider-session metadata into the typed request shape", () => {
    expect(
      toProviderSessionDetailRef({
        id: " sess_alpha ",
        kind: " SESSION_ID ",
        provider: " CoDeX ",
      }),
    ).toEqual({
      id: "sess_alpha",
      kind: "session_id",
      provider: "codex",
    });
  });

  it("rejects unsupported or unsafe provider-session metadata before issuing a request", () => {
    expect(
      toProviderSessionDetailRef({
        id: "../sess_alpha",
        kind: "session_id",
        provider: "codex",
      }),
    ).toBeNull();
    expect(
      toProviderSessionDetailRef({
        id: "sess_alpha",
        kind: "path",
        provider: "codex",
      }),
    ).toBeNull();
    expect(
      toProviderSessionDetailRef({
        id: "sess_alpha",
        kind: "session_id",
        provider: "anthropic",
      }),
    ).toBeNull();
  });
});
