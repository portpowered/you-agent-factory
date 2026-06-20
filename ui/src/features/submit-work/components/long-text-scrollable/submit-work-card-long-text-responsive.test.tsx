import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { getSubmitWorkMessages } from "../../messages/submit-work";
import {
  SUBMIT_WORK_LONG_TEXT_SCROLLABLE_FIXTURE,
  SubmitWorkCardLongTextScrollableVerification,
} from "./submit-work-card-long-text-scrollable-verification";
import { SubmitWorkCard } from "../submit-work-card";

const messages = getSubmitWorkMessages("en");
const defaultDraft = {
  items: [
    {
      id: "submission-item-1",
      text: SUBMIT_WORK_LONG_TEXT_SCROLLABLE_FIXTURE,
      type: "text" as const,
    },
  ],
  requestName: "Long text scroll verification",
  workTypeName: "story",
};

const VIEWPORT_WIDTHS = [
  { label: "small", width: 360 },
  { label: "medium", width: 768 },
  { label: "large", width: 1440 },
] as const;

function renderSubmitWorkCardAtWidth(
  width: number,
  overrides: Partial<ComponentProps<typeof SubmitWorkCard>> = {},
) {
  return render(
    <div style={{ width }}>
      <SubmitWorkCard
        draft={defaultDraft}
        onAddItem={() => {}}
        onItemTextChange={() => {}}
        onRemoveItem={() => {}}
        onRequestNameChange={() => {}}
        onStageFileItems={() => {}}
        onSubmit={() => {}}
        onWorkTypeNameChange={() => {}}
        status={{
          kind: "guidance",
          message: messages.statusMessages.ready,
        }}
        submitWorkTypeNames={["story", "task"]}
        {...overrides}
      />
    </div>,
  );
}

describe("SubmitWorkCard long text responsive layout", () => {
  afterEach(() => {
    cleanup();
  });

  for (const viewport of VIEWPORT_WIDTHS) {
    it(`keeps long text constrained and scrollable at ${viewport.label} widths`, () => {
      renderSubmitWorkCardAtWidth(viewport.width);

      const submissionTextarea = screen.getByRole<HTMLTextAreaElement>("textbox", {
        name: messages.requestItemLabel(1),
      });

      expect(submissionTextarea.className).toContain("max-h-52");
      expect(submissionTextarea.className).toContain("overflow-y-auto");
      expect(submissionTextarea.className).toContain("af-styled-scrollbar");

      Object.defineProperty(submissionTextarea, "scrollHeight", {
        configurable: true,
        value: 640,
      });
      Object.defineProperty(submissionTextarea, "clientHeight", {
        configurable: true,
        value: 208,
      });

      expect(submissionTextarea.scrollHeight).toBeGreaterThan(
        submissionTextarea.clientHeight,
      );
      submissionTextarea.scrollTop = 96;
      expect(submissionTextarea.scrollTop).toBe(96);
      expect(
        screen.getByRole("button", { name: messages.submitAction }),
      ).toBeVisible();
    });
  }

  it("supports keyboard navigation from the textarea to submit", async () => {
    const user = userEvent.setup();
    render(<SubmitWorkCardLongTextScrollableVerification />);

    const submissionTextarea = screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: messages.requestItemLabel(1),
    });
    const submitButton = screen.getByRole("button", {
      name: messages.submitAction,
    });

    submissionTextarea.focus();
    expect(submissionTextarea).toHaveFocus();

    Object.defineProperty(submissionTextarea, "scrollHeight", {
      configurable: true,
      value: 640,
    });
    Object.defineProperty(submissionTextarea, "clientHeight", {
      configurable: true,
      value: 208,
    });

    fireEvent.keyDown(submissionTextarea, { key: "PageDown" });
    submissionTextarea.scrollTop = 72;

    let focusedSubmit = false;
    for (let attempt = 0; attempt < 8; attempt += 1) {
      await user.tab();
      if (submitButton === document.activeElement) {
        focusedSubmit = true;
        break;
      }
    }

    expect(focusedSubmit).toBe(true);
  });
});
