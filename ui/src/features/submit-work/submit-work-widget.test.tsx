import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { SubmitWorkWidget } from "./submit-work-widget";

describe("SubmitWorkWidget", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("disables submission until a configured work type and request text are present", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget
        submitWorkTypes={[
          { work_type_name: "story" },
          { work_type_name: "task" },
        ]}
      />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", { name: "Work type" });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", { name: "Request name" });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Request" });
    const submitButton = screen.getByRole<HTMLButtonElement>("button", { name: "Submit work" });

    expect(submitButton.disabled).toBe(true);
    expect(
      screen.getByText("Choose a work type and describe what you need to get started."),
    ).toBeTruthy();

    fireEvent.change(workType, { target: { value: "story" } });
    expect(submitButton.disabled).toBe(true);
    expect(screen.getByText("Describe what you need to continue.")).toBeTruthy();

    fireEvent.change(requestName, { target: { value: "Driver review" } });
    expect(submitButton.disabled).toBe(true);

    fireEvent.change(requestText, { target: { value: "Review the failed driver trace." } });

    expect(submitButton.disabled).toBe(false);
    expect(screen.getByText("Your request is ready to submit.")).toBeTruthy();
  });

  it("shows inline validation and skips the network request when the draft is incomplete", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const submitButton = screen.getByRole<HTMLButtonElement>("button", { name: "Submit work" });
    const form = submitButton.closest("form");

    if (!(form instanceof HTMLFormElement)) {
      throw new Error("expected the submit button to be rendered inside a form");
    }

    fireEvent.submit(form);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(
      await screen.findByText("Choose a work type and describe your request before submitting."),
    ).toBeTruthy();
    expect(screen.getByText("Choose a work type before submitting.")).toBeTruthy();
    expect(screen.getByText("Describe your request before submitting.")).toBeTruthy();
  });

  it("submits work with an optional request name, clears the form on success, and shows the returned trace", async () => {
    const pendingResponse = {
      resolve: null as ((value: Response) => void) | null,
    };
    const fetchMock = vi.fn().mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          pendingResponse.resolve = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", { name: "Work type" });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", { name: "Request name" });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Request" });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, { target: { value: "Driver incident review" } });
    fireEvent.change(requestText, { target: { value: "Review the queue and summarize the failure." } });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    await waitFor(() => {
      expect(screen.getByRole<HTMLButtonElement>("button", { name: "Submitting..." })).toBeTruthy();
    });
    expect(screen.getByRole("button", { name: "Submitting..." }).getAttribute("aria-busy")).toBe(
      "true",
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/work");
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
    });
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      name: "Driver incident review",
      payload: "Review the queue and summarize the failure.",
      workTypeName: "story",
    });

    if (!pendingResponse.resolve) {
      throw new Error("expected submission to create a pending fetch promise");
    }

    pendingResponse.resolve(
      new Response(JSON.stringify({ trace_id: "trace-submit-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
    );

    expect(
      await screen.findByText("Your request was submitted. Trace ID: trace-submit-story."),
    ).toBeTruthy();
    expect(workType.value).toBe("");
    expect(requestName.value).toBe("");
    expect(requestText.value).toBe("");
  });

  it("omits the request name when the field is blank", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ trace_id: "trace-submit-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", { name: "Work type" });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Request" });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestText, { target: { value: "Review the queue and summarize the failure." } });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    await screen.findByText("Your request was submitted. Trace ID: trace-submit-story.");
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      payload: "Review the queue and summarize the failure.",
      workTypeName: "story",
    });
  });

  it("shows the server error inline and preserves the draft after a failed submission", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ code: "BAD_REQUEST", message: "work_type_name is required" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 400,
        statusText: "Bad Request",
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", { name: "Work type" });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", { name: "Request name" });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Request" });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, { target: { value: "Retry dashboard request" } });
    fireEvent.change(requestText, { target: { value: "Retry the broken submission." } });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    expect(await screen.findByText("work_type_name is required")).toBeTruthy();
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe("Retry the broken submission.");
  });

  it("renders an explained disabled state when no submit work types are configured", () => {
    renderSubmitWorkWidget(<SubmitWorkWidget submitWorkTypes={[]} />);

    const workType = screen.getByRole<HTMLSelectElement>("combobox", { name: "Work type" });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", { name: "Request name" });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Request" });
    const submitButton = screen.getByRole<HTMLButtonElement>("button", { name: "Submit work" });

    expect(workType.disabled).toBe(true);
    expect(requestName.disabled).toBe(true);
    expect(requestText.disabled).toBe(true);
    expect(submitButton.disabled).toBe(true);
    expect(
      screen.getByText("No work types are available to submit right now."),
    ).toBeTruthy();
  });
});

function renderSubmitWorkWidget(element: React.ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: {
        retry: false,
      },
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(<QueryClientProvider client={queryClient}>{element}</QueryClientProvider>);
}

