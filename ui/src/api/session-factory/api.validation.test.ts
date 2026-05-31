import { getSessionFactory } from "./api";

describe("getSessionFactory version validation", () => {
  it("accepts numeric logical version values from the API", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Current Factory",
              workers: [],
              workstations: [],
              workTypes: [],
              version: {
                logical: 42,
                physical: "2026-05-18T14:25:00Z",
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
        ),
      }),
    ).resolves.toMatchObject({
      name: "Current Factory",
      version: {
        logical: "42",
        physical: "2026-05-18T14:25:00Z",
      },
    });
  });

  it("throws when version physical is not a string", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Current Factory",
              workers: [],
              workstations: [],
              workTypes: [],
              version: {
                logical: "1",
                physical: 123,
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
        ),
      }),
    ).rejects.toMatchObject({
      code: "INVALID_FACTORY",
      name: "SessionFactoryAPIError",
    });
  });

  it("throws when factory version object is missing", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              name: "Current Factory",
              workers: [],
              workstations: [],
              workTypes: [],
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
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      name: "SessionFactoryAPIError",
    });
  });
});
