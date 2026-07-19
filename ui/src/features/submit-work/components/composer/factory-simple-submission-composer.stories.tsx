import { useState } from "react";

import { FactorySimpleSubmissionComposer } from "./factory-simple-submission-composer";

function SimpleComposerStory({ isCurrent = true }: { isCurrent?: boolean }) {
  const [draft, setDraft] = useState("Review the current queue.");

  return (
    <div className="w-full max-w-2xl p-4">
      <FactorySimpleSubmissionComposer
        draft={draft}
        factoryState="active"
        isCurrent={isCurrent}
        onDraftChange={(value) => {
          setDraft(value);
        }}
        onSubmit={async () => {
          if (draft.includes("fail")) {
            throw new Error("The host could not submit the work.");
          }
        }}
        workTypes={[
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: true,
            name: "task",
          },
        ]}
      />
    </div>
  );
}

export default {
  title: "Agent Factory/Submit Work/Factory Simple Submission Composer",
  component: FactorySimpleSubmissionComposer,
};

export const Interactive = {
  render: () => <SimpleComposerStory />,
};

export const HistoryUnavailable = {
  render: () => <SimpleComposerStory isCurrent={false} />,
};
