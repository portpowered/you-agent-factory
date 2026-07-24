import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { DashboardSessionTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { FactoryInvocationWidget } from "./factory-invocation-widget";

const invokeSessionFactory = vi.fn();
const useCurrentFactoryDefinition = vi.fn();
let restoreBrowserShims: (() => void) | undefined;

vi.mock("../../../../api/session-factory", async () => {
  const actual = (await vi.importActual(
    "../../../../api/session-factory",
  )) as typeof import("../../../../api/session-factory");

  return {
    ...actual,
    invokeSessionFactory: (...args: unknown[]) => invokeSessionFactory(...args),
  };
});

vi.mock("../../../current-factory-definition/public", async () => {
  const actual = (await vi.importActual(
    "../../../current-factory-definition/public",
  )) as typeof import("../../../current-factory-definition/public");

  return {
    ...actual,
    useCurrentFactoryDefinition: () => useCurrentFactoryDefinition(),
  };
});

describe("FactoryInvocationWidget extra rendering coverage", () => {
  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    invokeSessionFactory.mockReset();
    useCurrentFactoryDefinition.mockReset();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("supports clearing optional boolean arguments and removing repeated rows", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "NAMED" }],
              name: "confirm",
              typeHint: "BOOLEAN_STRING",
            },
            {
              bindings: [{ kind: "NAMED" }],
              name: "tag",
              valueMode: "REPEATED",
            },
          ],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });
    invokeSessionFactory.mockResolvedValue({
      requestId: "request-1",
      status: "COMPLETED",
      traceId: "trace-1",
    });

    renderFactoryInvocationWidget();

    await user.click(screen.getByRole("button", { name: "True" }));
    await user.click(screen.getByRole("button", { name: "Use default" }));
    await user.type(screen.getByRole("textbox", { name: /tag/i }), "alpha");
    await user.click(screen.getByRole("button", { name: "Add tag" }));
    await user.type(
      screen.getAllByRole("textbox", { name: /tag/i })[1],
      "beta",
    );
    await user.click(
      screen.getByRole("button", { name: "Remove tag value 2" }),
    );
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(invokeSessionFactory).toHaveBeenCalledWith(
        {
          args: {
            tag: ["alpha"],
          },
        },
        { sessionID: "~default" },
      );
    });
  });

  it("shows a generic primary-result state when no text parts are present", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });
    invokeSessionFactory.mockResolvedValue({
      primaryResult: [
        { kind: "image", uri: "https://example.test/result.png" },
      ] as never,
      requestId: "request-1",
      status: "COMPLETED",
      traceId: "trace-1",
    });

    renderFactoryInvocationWidget();

    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(screen.getByText("Primary result")).toBeVisible();
    });
  });
});

function renderFactoryInvocationWidget() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DashboardSessionTestProvider>
        <FactoryInvocationWidget sessionID="~default" />
      </DashboardSessionTestProvider>
    </QueryClientProvider>,
  );
}
