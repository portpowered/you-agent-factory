import {
  getPackagedFactoryCatalog,
  type PackagedFactoryCatalogAPIError,
} from "./api";

const catalog = {
  factories: [
    {
      name: "@you/goal",
      project: "builtin-goal",
      slug: "goal",
      description: {
        type: "LOCALIZABLE_ASSET" as const,
        value: "Turn a goal into bounded work.",
      },
      examples: [
        {
          name: "default",
          description: {
            type: "LOCALIZABLE_ASSET" as const,
            value: "Run a goal.",
          },
          args: { goal: "Ship the feature." },
        },
      ],
      json: { id: "builtin-goal", name: "goal" },
      yaml: "id: builtin-goal\nname: goal\n",
    },
  ],
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
    status,
    statusText: status === 200 ? "OK" : "Internal Server Error",
  });
}

describe("getPackagedFactoryCatalog", () => {
  it("reads the generated backend contract, including an empty catalog", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ factories: [] }))
      .mockResolvedValueOnce(jsonResponse(catalog));

    await expect(
      getPackagedFactoryCatalog({ fetch: fetchMock }),
    ).resolves.toEqual({ factories: [] });
    await expect(
      getPackagedFactoryCatalog({ fetch: fetchMock }),
    ).resolves.toEqual(catalog);
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      "/packaged-factories",
      expect.objectContaining({
        headers: { Accept: "application/json" },
        method: "GET",
        signal: undefined,
      }),
    );
  });

  it("rejects malformed success payloads at the typed API boundary", async () => {
    await expect(
      getPackagedFactoryCatalog({
        fetch: vi.fn().mockResolvedValue(jsonResponse({ factories: [{}] })),
      }),
    ).rejects.toMatchObject<Partial<PackagedFactoryCatalogAPIError>>({
      code: "INTERNAL_ERROR",
      status: 200,
    });
  });

  it("maps backend and network failures without making raw content part of the UI contract", async () => {
    await expect(
      getPackagedFactoryCatalog({
        fetch: vi
          .fn()
          .mockResolvedValue(jsonResponse({ code: "INTERNAL_ERROR" }, 500)),
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      responseBody: { code: "INTERNAL_ERROR" },
      status: 500,
    });

    await expect(
      getPackagedFactoryCatalog({
        fetch: vi.fn().mockRejectedValue(new Error("connection reset")),
      }),
    ).rejects.toMatchObject({
      code: "NETWORK_ERROR",
    });
  });
});
