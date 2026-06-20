import { getSubmitWorkMessages } from "../../messages/submit-work";
import { SubmitWorkCard } from "../submit-work-card";

const verificationMessages = getSubmitWorkMessages("en");

export const SUBMIT_WORK_LONG_TEXT_SCROLLABLE_FIXTURE =
  "Review line of submitted work content for scroll verification.\n".repeat(32);

export function SubmitWorkCardLongTextScrollableVerification() {
  return (
    <SubmitWorkCard
      draft={{
        items: [
          {
            id: "submission-item-1",
            text: SUBMIT_WORK_LONG_TEXT_SCROLLABLE_FIXTURE,
            type: "text",
          },
        ],
        requestName: "Long text scroll verification",
        workTypeName: "story",
      }}
      onAddItem={() => {}}
      onItemTextChange={() => {}}
      onRemoveItem={() => {}}
      onRequestNameChange={() => {}}
      onStageFileItems={() => {}}
      onSubmit={() => {}}
      onWorkTypeNameChange={() => {}}
      status={{
        kind: "guidance",
        message: verificationMessages.statusMessages.ready,
      }}
      submitWorkTypeNames={["story", "task"]}
      widgetId="submit-work-long-text-scrollable-verification"
    />
  );
}
