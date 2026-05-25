import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { useDashboardSessionStore } from "../../dashboard/state/dashboardSessionStore";
import { useSubmitWorkWidget } from "../hooks/use-submit-work-widget";
import { getSubmitWorkMessages } from "../messages/submit-work";
import { SubmitWorkCard } from "./submit-work-card";
import { SubmitWorkWidget } from "./submit-work-widget";

describe("SubmitWorkWidget", () => {
  beforeEach(() => {
    useDashboardSessionStore.setState({
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps the submit-work card focused on the form without redundant helper copy", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget
        submitWorkTypes={[
          { work_type_name: "story" },
          { work_type_name: "task" },
        ]}
      />,
    );

    const card = screen.getByRole("article", { name: "Submit work" });

    expect(
      screen.queryByText(
        "Send a new request to the current factory from the dashboard.",
      ),
    ).toBeNull();
    expect(
      within(card).getByRole("combobox", { name: "Work type" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("textbox", { name: "Request name" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("list", { name: "Submission items" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("textbox", { name: "Text item 1" }),
    ).toBeTruthy();
    expect(
      within(card).getByText(
        "Choose a work type and enter a request name to continue.",
      ),
    ).toBeTruthy();
    expect(
      within(card).queryByText(
        "Optional: describe what you want this request to accomplish.",
      ),
    ).toBeNull();
    expect(
      within(card).getByRole("button", { name: "Submit work" }),
    ).toBeTruthy();
  });

  it("enables submission only after a configured work type and non-blank request name are present", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget
        submitWorkTypes={[
          { work_type_name: "story" },
          { work_type_name: "task" },
        ]}
      />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });
    const submitButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Submit work",
    });

    expect(submitButton.disabled).toBe(true);
    expect(
      screen.getByText(
        "Choose a work type and enter a request name to continue.",
      ),
    ).toBeTruthy();

    fireEvent.change(workType, { target: { value: "story" } });
    expect(submitButton.disabled).toBe(true);
    expect(screen.getByText("Enter a request name to continue.")).toBeTruthy();

    fireEvent.change(requestName, { target: { value: "   " } });
    expect(submitButton.disabled).toBe(true);

    fireEvent.change(requestName, { target: { value: "Driver review" } });
    expect(submitButton.disabled).toBe(false);

    fireEvent.change(requestText, {
      target: { value: "Review the failed driver trace." },
    });

    expect(submitButton.disabled).toBe(false);
    expect(screen.getByText("Ready to submit.")).toBeTruthy();
    expect(
      screen.queryByText("Ready to submit. Request details are optional."),
    ).toBeNull();
    expect(submitButton.className).toContain("bg-af-accent");
  });

  it("shows inline validation and skips the network request when the draft is incomplete", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const submitButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Submit work",
    });
    const form = submitButton.closest("form");

    if (!(form instanceof HTMLFormElement)) {
      throw new Error(
        "expected the submit button to be rendered inside a form",
      );
    }

    fireEvent.submit(form);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(
      await screen.findAllByText(
        "Choose a work type and enter a request name before submitting.",
      ),
    ).toHaveLength(1);
    expect(
      screen.getByText("Choose a work type before submitting."),
    ).toBeTruthy();
    expect(
      screen.getByText("Enter a request name before submitting."),
    ).toBeTruthy();
  });

  it("renders a seeded ordered submission-items list with one blank text item by default", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const submissionItems = screen.getByRole<HTMLOListElement>("list", {
      name: "Submission items",
    });
    const seededTextItem = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    expect(within(submissionItems).getAllByRole("listitem")).toHaveLength(1);
    expect(seededTextItem.value).toBe("");
    expect(screen.getByText("Text")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "Add input",
      }),
    ).toBeTruthy();
  });

  it("adds typed items from the shared add-input control and renders their type cues", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: "Add input",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "Image",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "Add input",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "Text",
      }),
    );

    const submissionItems = screen.getByRole<HTMLOListElement>("list", {
      name: "Submission items",
    });

    expect(within(submissionItems).getAllByRole("listitem")).toHaveLength(3);
    expect(screen.getByText("Image")).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Text item 3" })).toBeTruthy();
  });

  it("removes only the targeted item and restores one blank text item when the last item is removed", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: "Add input",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "Image",
      }),
    );

    const originalTextItem = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });
    fireEvent.change(originalTextItem, {
      target: { value: "Keep this text item." },
    });

    fireEvent.click(
      screen.getByRole("button", {
        name: "Remove image item 2",
      }),
    );

    expect(screen.queryByText("Image")).toBeNull();
    expect(
      screen.getByRole<HTMLTextAreaElement>("textbox", {
        name: "Text item 1",
      }).value,
    ).toBe("Keep this text item.");

    fireEvent.click(
      screen.getByRole("button", {
        name: "Remove text item 1",
      }),
    );

    const fallbackSubmissionItems = screen.getByRole<HTMLOListElement>("list", {
      name: "Submission items",
    });
    const fallbackTextItem = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    expect(within(fallbackSubmissionItems).getAllByRole("listitem")).toHaveLength(1);
    expect(fallbackTextItem.value).toBe("");
  });

  it("renders drag-active and ready file states through the shared upload primitive", async () => {
    const view = render(
      <SubmitWorkCard
        draft={{
          items: [{ id: "submission-item-2", stagingStatus: "idle", type: "image" }],
          requestName: "Image review",
          workTypeName: "story",
        }}
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{ kind: "guidance", message: "Stage each file-backed item before submitting." }}
        submitWorkTypeNames={["story"]}
      />,
    );

    const dropzoneLabel = screen.getByText("Image file");
    const dropzone = dropzoneLabel.closest("label");
    if (!(dropzone instanceof HTMLLabelElement)) {
      throw new Error("expected image upload dropzone label");
    }
    fireEvent.dragOver(dropzone, {
      dataTransfer: {
        dropEffect: "copy",
        files: [],
        types: ["Files"],
      },
    });
    expect(screen.getByText("Drop the image file to stage it.")).toBeTruthy();

    view.unmount();
    render(
      <SubmitWorkCard
        draft={{
          items: [
            {
              fileName: "ui.png",
              id: "submission-item-2",
              mediaType: "image/png",
              stagedFileRef: "/tmp/submit-work-stage/ui.png",
              stagingStatus: "ready",
              type: "image",
            },
          ],
          requestName: "Image review",
          workTypeName: "story",
        }}
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{ kind: "guidance", message: "Ready to submit." }}
        submitWorkTypeNames={["story"]}
      />,
    );

    expect(screen.getByText("ui.png (image/png)")).toBeTruthy();
    expect(screen.getByText("Replace file")).toBeTruthy();
  });

  it("renders file-staging failures as item-scoped errors and keeps submit disabled", () => {
    render(
      <SubmitWorkCard
        draft={{
          items: [
            {
              fileName: "ui.png",
              id: "submission-item-2",
              mediaType: "application/pdf",
              stagingError: "mediaType must start with image/ for image items",
              stagingStatus: "failure",
              type: "image",
            },
          ],
          requestName: "Image review",
          workTypeName: "story",
        }}
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{ kind: "guidance", message: "Stage each file-backed item before submitting." }}
        submitWorkTypeNames={["story"]}
      />,
    );

    expect(
      screen.getByText("mediaType must start with image/ for image items"),
    ).toBeTruthy();
    expect(
      screen.getByText("Retry staging this image file or choose a different file."),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Submit work" }).disabled).toBe(
      true,
    );
  });

  it("stages multiple selected files as independent ordered sibling items and removes them independently", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            fileName: "first.png",
            mediaType: "image/png",
            stagedFileRef: "/tmp/submit-work-stage/first.png",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 201,
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            fileName: "second.png",
            mediaType: "image/png",
            stagedFileRef: "/tmp/submit-work-stage/second.png",
          }),
          {
            headers: {
              "Content-Type": "application/json",
            },
            status: 201,
          },
        ),
      );
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(
      () =>
        useSubmitWorkWidget(
          DEFAULT_FACTORY_SESSION_ID,
          [{ work_type_name: "story" }],
          getSubmitWorkMessages("en"),
        ),
      {
        wrapper: ({ children }) => (
          <QueryClientProvider client={new QueryClient()}>
            {children}
          </QueryClientProvider>
        ),
      },
    );

    act(() => {
      result.current.onAddItem("image");
    });
    await waitFor(() => {
      expect(result.current.draft.items).toHaveLength(2);
    });

    const firstFile = createStageableFile("first", "first.png", "image/png");
    const secondFile = createStageableFile("second", "second.png", "image/png");

    await act(async () => {
      await result.current.onStageFileItems("submission-item-2", [
        firstFile,
        secondFile,
      ]);
    });

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    await waitFor(() => {
      expect(result.current.draft.items).toEqual([
        { id: "submission-item-1", text: "", type: "text" },
        {
          fileName: "first.png",
          id: "submission-item-2",
          mediaType: "image/png",
          stagedFileRef: "/tmp/submit-work-stage/first.png",
          stagingError: undefined,
          stagingStatus: "ready",
          type: "image",
        },
        {
          fileName: "second.png",
          id: "submission-item-3",
          mediaType: "image/png",
          stagedFileRef: "/tmp/submit-work-stage/second.png",
          stagingError: undefined,
          stagingStatus: "ready",
          type: "image",
        },
      ]);
    });
    expect(
      fetchMock.mock.calls.map((call) =>
        JSON.parse(String((call[1] as RequestInit).body)).fileName,
      ),
    ).toEqual(["first.png", "second.png"]);

    act(() => {
      result.current.onRemoveItem("submission-item-2");
    });

    expect(result.current.draft.items).toEqual([
      { id: "submission-item-1", text: "", type: "text" },
      {
        fileName: "second.png",
        id: "submission-item-3",
        mediaType: "image/png",
        stagedFileRef: "/tmp/submit-work-stage/second.png",
        stagingError: undefined,
        stagingStatus: "ready",
        type: "image",
      },
    ]);
  });

  it("submits work with a request name, clears only request fields on success, and shows the returned trace", async () => {
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

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Driver incident review" },
    });
    fireEvent.change(requestText, {
      target: { value: "Review the queue and summarize the failure." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    await waitFor(() => {
      expect(
        screen.getByRole<HTMLButtonElement>("button", {
          name: "Submitting...",
        }),
      ).toBeTruthy();
    });
    const submittingButton = screen.getByRole("button", {
      name: "Submitting...",
    });
    const submittingStatus = screen.getByText("Sending your request...");
    expect(submittingButton.getAttribute("aria-busy")).toBe("true");
    expect(screen.getByRole("status").textContent).toBe(
      "Sending your request...",
    );
    expect(submittingStatus.className).toContain("text-af-text");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/work`,
    );
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: "POST",
    });
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      items: [
        {
          text: "Review the queue and summarize the failure.",
          type: "text",
        },
      ],
      name: "Driver incident review",
      workTypeName: "story",
    });

    if (!pendingResponse.resolve) {
      throw new Error("expected submission to create a pending fetch promise");
    }

    pendingResponse.resolve(
      new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
        headers: {
          "Content-Type": "application/json",
        },
        status: 201,
      }),
    );

    expect(
      await screen.findByText(
        "Your request was submitted. Trace ID: trace-submit-story.",
      ),
    ).toBeTruthy();
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("");
    expect(requestText.value).toBe("");
  });

  it("clears a preserved work type when the configured options no longer include it", async () => {
    const { rerender } = renderSubmitWorkWidget(
      <SubmitWorkWidget
        submitWorkTypes={[
          { work_type_name: "story" },
          { work_type_name: "task" },
        ]}
      />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    expect(workType.value).toBe("story");

    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "task" }]} />
      </QueryClientProvider>,
    );

    expect(
      screen.getByRole<HTMLSelectElement>("combobox", {
        name: "Work type",
      }).value,
    ).toBe("");
  });

  it("shows inline request-name validation and skips the network request when the name is blank", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
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

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const submitButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Submit work",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    expect(submitButton.disabled).toBe(true);

    fireEvent.change(requestName, { target: { value: "   " } });
    expect(submitButton.disabled).toBe(true);

    const form = submitButton.closest("form");
    if (!(form instanceof HTMLFormElement)) {
      throw new Error(
        "expected the submit button to be rendered inside a form",
      );
    }

    fireEvent.submit(form);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(
      await screen.findAllByText("Enter a request name before submitting."),
    ).toHaveLength(2);
    expect(requestName.getAttribute("aria-invalid")).toBe("true");
  });

  it("blocks obviously empty submissions before the network request", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Empty payload request" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    expect(
      await screen.findAllByText(
        "Add at least one non-empty text item or one staged file before submitting.",
      ),
    ).toHaveLength(2);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("preserves authored text-item order in the structured submit request", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
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

    fireEvent.click(
      screen.getByRole("button", {
        name: "Add input",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "Text",
      }),
    );

    fireEvent.change(screen.getByRole("combobox", { name: "Work type" }), {
      target: { value: "story" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Request name" }), {
      target: { value: "Ordered text payload" },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Text item 1" }), {
      target: { value: "First authored part." },
    });
    fireEvent.change(screen.getByRole("textbox", { name: "Text item 2" }), {
      target: { value: "Second authored part." },
    });

    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });
    expect(JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body))).toEqual({
      items: [
        {
          text: "First authored part.",
          type: "text",
        },
        {
          text: "Second authored part.",
          type: "text",
        },
      ],
      name: "Ordered text payload",
      workTypeName: "story",
    });
  });

  it("shows the server error inline and preserves the draft after a failed submission", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "BAD_REQUEST",
          message: "work_type_name is required",
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 400,
          statusText: "Bad Request",
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderSubmitWorkWidget(
      <SubmitWorkWidget submitWorkTypes={[{ work_type_name: "story" }]} />,
    );

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Retry dashboard request" },
    });
    fireEvent.change(requestText, {
      target: { value: "Retry the broken submission." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    expect(await screen.findByText("work_type_name is required")).toBeTruthy();
    expect(workType.value).toBe("story");
    expect(requestName.value).toBe("Retry dashboard request");
    expect(requestText.value).toBe("Retry the broken submission.");
  });

  it("clears the draft and switches submit routing when the selected session changes", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ traceId: "trace-submit-story" }), {
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

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Switch session draft" },
    });
    fireEvent.change(requestText, {
      target: { value: "Do not leak this into another tab." },
    });

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    await waitFor(() => {
      expect(workType.value).toBe("");
    });
    expect(requestName.value).toBe("");
    expect(requestText.value).toBe("");

    fireEvent.change(workType, { target: { value: "story" } });
    fireEvent.change(requestName, {
      target: { value: "Beta submission" },
    });
    fireEvent.change(requestText, {
      target: { value: "Submit the beta session request." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Submit work" }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        "/factory-sessions/session-beta/work",
        expect.objectContaining({
          method: "POST",
        }),
      );
    });
  });

  it("renders an explained disabled state when no submit work types are configured", () => {
    renderSubmitWorkWidget(<SubmitWorkWidget submitWorkTypes={[]} />);

    const workType = screen.getByRole<HTMLSelectElement>("combobox", {
      name: "Work type",
    });
    const requestName = screen.getByRole<HTMLInputElement>("textbox", {
      name: "Request name",
    });
    const requestText = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Text item 1",
    });
    const submitButton = screen.getByRole<HTMLButtonElement>("button", {
      name: "Submit work",
    });

    expect(workType.disabled).toBe(true);
    expect(requestName.disabled).toBe(true);
    expect(requestText.disabled).toBe(true);
    expect(submitButton.disabled).toBe(true);
    expect(
      screen.getByText("No work types are available to submit right now."),
    ).toBeTruthy();
  });

  it("renders zh-CN copy for the submit-work form shell", () => {
    renderSubmitWorkWidget(
      <SubmitWorkWidget
        locale="zh-CN"
        submitWorkTypes={[{ work_type_name: "story" }]}
      />,
    );

    const card = screen.getByRole("article", { name: "提交工作" });

    expect(
      within(card).getByRole("combobox", { name: "工作类型" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("textbox", { name: "请求名称" }),
    ).toBeTruthy();
    expect(within(card).getByRole("list", { name: "提交项" })).toBeTruthy();
    expect(within(card).getByRole("textbox", { name: "文本项 1" })).toBeTruthy();
    expect(
      within(card).getByText("先选择工作类型并填写请求名称，然后即可继续。"),
    ).toBeTruthy();
    expect(within(card).getByRole("button", { name: "提交工作" })).toBeTruthy();
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

  return render(
    <QueryClientProvider client={queryClient}>{element}</QueryClientProvider>,
  );
}

function createStageableFile(
  content: string,
  fileName: string,
  mediaType: string,
): File {
  const file = new File([content], fileName, { type: mediaType });
  Object.defineProperty(file, "arrayBuffer", {
    value: async () => new TextEncoder().encode(content).buffer,
  });
  return file;
}
