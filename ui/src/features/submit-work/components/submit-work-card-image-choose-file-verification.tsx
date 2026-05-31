import { useState } from "react";

import { getSubmitWorkMessages } from "../messages/submit-work";
import type { SubmitWorkDraftFileItem } from "./submit-work-card";
import { SubmitWorkCard } from "./submit-work-card";

const verificationMessages = getSubmitWorkMessages("en");

export function SubmitWorkCardImageChooseFileVerification() {
  const [items, setItems] = useState<SubmitWorkDraftFileItem[]>([
    { id: "submission-item-2", stagingStatus: "idle", type: "image" },
  ]);

  return (
    <SubmitWorkCard
      draft={{
        items,
        requestName: "Image review",
        workTypeName: "story",
      }}
      onAddItem={() => {}}
      onItemTextChange={() => {}}
      onRemoveItem={() => {}}
      onRequestNameChange={() => {}}
      onStageFileItems={(itemId, files) => {
        const nextFile = files[0];
        if (!nextFile) {
          return;
        }

        setItems((currentItems) =>
          currentItems.map((item) =>
            item.id === itemId
              ? {
                  ...item,
                  fileName: nextFile.name,
                  mediaType: nextFile.type,
                  stagingStatus: "ready",
                }
              : item,
          ),
        );
      }}
      onSubmit={() => {}}
      onWorkTypeNameChange={() => {}}
      status={{
        kind: "guidance",
        message: verificationMessages.statusMessages.fileItemsNeedAttention,
      }}
      submitWorkTypeNames={["story"]}
      widgetId="submit-work-image-choose-file-verification"
    />
  );
}
