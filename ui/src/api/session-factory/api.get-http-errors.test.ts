import { getSessionFactory } from "./api";
import { sessionFactoryAPIErrorMessages } from "./messages";
import { sessionFactoryOperatorErrorMessages } from "./operator-errors";

describe("getSessionFactory HTTP error mapping", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("maps unrecognized API error codes on 4xx responses to INTERNAL_ERROR with API copy", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "UNKNOWN_CODE",
              message: "Unexpected failure.",
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
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: "Unexpected failure.",
      status: 400,
    });
  });

  it("maps unrecognized API error codes on 5xx responses to rejected request copy", async () => {
    await expect(
      getSessionFactory("~default", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "UNKNOWN_CODE",
              message: "Unexpected failure.",
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
      }),
    ).rejects.toMatchObject({
      code: "INTERNAL_ERROR",
      message: sessionFactoryAPIErrorMessages.rejectedRequest,
      status: 500,
    });
  });

  it.each([
    ["INVALID_FACTORY", sessionFactoryOperatorErrorMessages.INVALID_FACTORY],
    [
      "INVALID_FACTORY_NAME",
      sessionFactoryOperatorErrorMessages.INVALID_FACTORY_NAME,
    ],
  ] as const)(
    "maps %s GET failures to canonical operator copy",
    async (code, message) => {
      await expect(
        getSessionFactory("~default", {
          fetch: vi.fn().mockResolvedValue(
            new Response(
              JSON.stringify({
                code,
                message: "Ignored API diagnostic.",
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
        }),
      ).rejects.toMatchObject({
        code,
        message,
        status: 400,
      });
    },
  );
});
