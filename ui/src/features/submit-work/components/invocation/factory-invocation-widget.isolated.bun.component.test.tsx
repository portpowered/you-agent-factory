// Isolated because Bun module mocks are process-global.
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, mock } from "bun:test";
import userEvent from "@testing-library/user-event";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { bunVi as vi } from "../../../../testing/bun/vi-compat";
import { DashboardSessionTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { selectLabeledComboboxOption } from "../../../../testing/select-test-helpers";

const invokeSessionFactory = vi.fn();
const useCurrentFactoryDefinition = vi.fn();
let restoreBrowserShims: (() => void) | undefined;

const actualSessionFactory = await import("../../../../api/session-factory");
const currentFactoryDefinitionHooks = await import(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition"
);
const { SessionFactoryInvocationError } = actualSessionFactory;

mock.module("../../../../api/session-factory", () => {
  return {
    ...actualSessionFactory,
    invokeSessionFactory: (...args: unknown[]) => invokeSessionFactory(...args),
  };
});

mock.module(
  "../../../current-factory-definition/hooks/useCurrentFactoryDefinition",
  () => ({
    ...currentFactoryDefinitionHooks,
    useCurrentFactoryDefinition: () => useCurrentFactoryDefinition(),
  }),
);

const { FactoryInvocationWidget } = await import(
  "./factory-invocation-widget"
);

/*
 * The API module remains real apart from the invocation edge. This keeps error
 * class behavior under test while avoiding the feature's public barrel.
 */
function resetInvocationMocks() {
  invokeSessionFactory.mockReset();
  useCurrentFactoryDefinition.mockReset();
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: focused widget coverage is kept together for the generated invocation flow.
describe("FactoryInvocationWidget", () => {
  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    resetInvocationMocks();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders signature-backed fields and submits InvocationRequest.args", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        examples: [
          {
            args: {
              input: "Draft a release summary",
              tags: ["release", "summary"],
            },
            description: {
              type: "LOCALIZABLE_ASSET",
              value: "Invoke fusion with structured input.",
            },
            name: "Positional input",
          },
        ],
        invocationSignature: {
          outputContract: {
            contentType: "text/markdown",
            description: "Writes the fused result to a markdown file.",
            fileExtension: ".md",
            mode: "FILE",
            pathParameter: "output",
          },
          parameters: [
            {
              aliases: ["body"],
              bindings: [
                { kind: "POSITIONAL", position: 1 },
                { kind: "STDIN" },
              ],
              description: "Source input",
              name: "input",
              required: true,
            },
            {
              bindings: [{ kind: "NAMED" }],
              choices: ["low", "medium", "high"],
              name: "effort",
            },
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
      primaryResult: [{ text: "Fusion complete." }],
      requestId: "request-1",
      status: "COMPLETED",
      traceId: "trace-1",
    });

    renderFactoryInvocationWidget();

    await user.type(
      screen.getByRole("textbox", { name: /input/i }),
      "hello world",
    );
    await selectLabeledComboboxOption(user, /effort/i, "high");
    await user.click(screen.getByRole("button", { name: "True" }));
    await user.type(screen.getByRole("textbox", { name: /tag/i }), "alpha");
    await user.click(screen.getByRole("button", { name: "Add tag" }));
    const tagInputs = screen.getAllByRole("textbox", { name: /tag/i });
    const secondTagInput = tagInputs[1];
    if (!(secondTagInput instanceof HTMLInputElement)) {
      throw new Error("expected a second repeated tag input");
    }
    await user.type(secondTagInput, "beta");
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(invokeSessionFactory).toHaveBeenCalledWith(
        {
          args: {
            confirm: "true",
            effort: "high",
            input: "hello world",
            tag: ["alpha", "beta"],
          },
        },
        { sessionID: "~default" },
      );
    });

    expect(screen.getByText("Output hint")).toBeVisible();
    expect(
      screen.getByText("Writes the fused result to a markdown file."),
    ).toBeVisible();
    expect(screen.getByText("Output mode: FILE")).toBeVisible();
    expect(screen.getByText("Output path argument: output")).toBeVisible();
    expect(screen.getByText("Content type: text/markdown")).toBeVisible();
    expect(screen.getByText("File extension: .md")).toBeVisible();
    expect(screen.getByText("Examples")).toBeVisible();
    expect(screen.getByText("Positional input")).toBeVisible();
    expect(
      screen.getByText("Invoke fusion with structured input."),
    ).toBeVisible();
    expect(
      screen.getByText(
        '{"input":"Draft a release summary","tags":["release","summary"]}',
      ),
    ).toBeVisible();
    expect(screen.getByText("Accepts stdin input.")).toBeVisible();
    expect(
      screen.getByText("Factory invocation started. Trace ID: trace-1."),
    ).toBeVisible();
    expect(screen.getByText("Primary result")).toBeVisible();
    expect(screen.getByText("Fusion complete.")).toBeVisible();
  });

  it("shows explicit loading and error states from the current-factory query", () => {
    useCurrentFactoryDefinition.mockReturnValue({
      data: undefined,
      error: null,
      isLoading: true,
    });

    const { rerender } = renderFactoryInvocationWidget();

    expect(
      screen.getByText("Loading the current factory invocation contract..."),
    ).toBeVisible();

    useCurrentFactoryDefinition.mockReturnValue({
      data: undefined,
      error: new Error("Current factory failed to load."),
      isLoading: false,
    });

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <DashboardSessionTestProvider>
          <FactoryInvocationWidget sessionID="~default" />
        </DashboardSessionTestProvider>
      </QueryClientProvider>,
    );

    expect(screen.getByText("Current factory failed to load.")).toBeVisible();
  });

  it("preserves explicit structured invocation when every signature field is omitted", async () => {
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
      requestId: "request-1",
      status: "COMPLETED",
      traceId: "trace-1",
    });

    renderFactoryInvocationWidget();

    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(invokeSessionFactory).toHaveBeenCalledWith(
        {
          args: {},
        },
        { sessionID: "~default" },
      );
    });
  });

  it("preserves the success message across a same-signature refresh rerender", async () => {
    const user = userEvent.setup();
    const signature = {
      parameters: [
        {
          bindings: [{ kind: "POSITIONAL", position: 1 }],
          name: "input",
          required: true,
        },
      ],
    };
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: signature,
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

    const { rerender } = renderFactoryInvocationWidget();

    await user.type(
      screen.getByRole("textbox", { name: /input/i }),
      "hello world",
    );
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(
        screen.getByText("Factory invocation started. Trace ID: trace-1."),
      ).toBeVisible();
    });

    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "POSITIONAL", position: 1 }],
              name: "input",
              required: true,
            },
          ],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <DashboardSessionTestProvider>
          <FactoryInvocationWidget sessionID="~default" />
        </DashboardSessionTestProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.getByText("Factory invocation started. Trace ID: trace-1."),
    ).toBeVisible();
    expect(screen.getByRole("textbox", { name: /input/i })).toHaveValue(
      "hello world",
    );
  });

  it("surfaces runtime failures returned in the invocation response", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "POSITIONAL", position: 1 }],
              name: "input",
              required: true,
            },
          ],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });
    invokeSessionFactory.mockResolvedValue({
      message: "Provider invocation failed.",
      requestId: "request-1",
      status: "FAILED",
      traceId: "trace-1",
    });

    renderFactoryInvocationWidget();

    await user.type(screen.getByRole("textbox", { name: /input/i }), "hello");
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(screen.getByText("Provider invocation failed.")).toBeVisible();
    });
  });

  it("surfaces field-level validation failures for required parameters", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "POSITIONAL", position: 1 }],
              name: "input",
              required: true,
            },
          ],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });

    renderFactoryInvocationWidget();

    await user.click(screen.getByRole("button", { name: "Run factory" }));

    expect(invokeSessionFactory).not.toHaveBeenCalled();
    expect(screen.getByText("Enter input before invoking.")).toBeVisible();
    expect(
      screen.getByText("Fix the highlighted arguments before invoking."),
    ).toBeVisible();
  });

  it("maps backend argument failures back onto the matching field", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "POSITIONAL", position: 1 }],
              name: "input",
              required: true,
            },
          ],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });
    invokeSessionFactory.mockRejectedValue(
      new SessionFactoryInvocationError(
        'required invocation parameter "input" is missing',
        {
          code: "INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT",
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );

    renderFactoryInvocationWidget();

    await user.type(screen.getByRole("textbox", { name: /input/i }), "hello");
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(
        screen.getAllByText('required invocation parameter "input" is missing')
          .length,
      ).toBeGreaterThan(0);
    });
  });

  it("surfaces the generic fallback for unexpected invocation errors", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "POSITIONAL", position: 1 }],
              name: "input",
              required: true,
            },
          ],
        },
        name: "fusion",
      },
      error: null,
      isLoading: false,
    });
    invokeSessionFactory.mockRejectedValue(new Error("network down"));

    renderFactoryInvocationWidget();

    await user.type(screen.getByRole("textbox", { name: /input/i }), "hello");
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(
        screen.getByText(
          "We couldn't invoke this factory. Try again in a moment.",
        ),
      ).toBeVisible();
    });
  });

  it("resets widget state when the session changes", async () => {
    const user = userEvent.setup();
    useCurrentFactoryDefinition.mockReturnValue({
      data: {
        invocationSignature: {
          parameters: [
            {
              bindings: [{ kind: "POSITIONAL", position: 1 }],
              name: "input",
              required: true,
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

    const { rerender } = renderFactoryInvocationWidget();

    await user.type(
      screen.getByRole("textbox", { name: /input/i }),
      "hello world",
    );
    await user.click(screen.getByRole("button", { name: "Run factory" }));

    await waitFor(() => {
      expect(
        screen.getByText("Factory invocation started. Trace ID: trace-1."),
      ).toBeVisible();
    });

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <DashboardSessionTestProvider>
          <FactoryInvocationWidget sessionID="session-beta" />
        </DashboardSessionTestProvider>
      </QueryClientProvider>,
    );

    expect(
      screen.queryByText("Factory invocation started. Trace ID: trace-1."),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /input/i })).toHaveValue("");
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
