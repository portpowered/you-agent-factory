export type RoleQueryOptions = {
  name?: string | RegExp;
};

export type SyncRoleQuery = {
  getByRole: <T extends HTMLElement = HTMLElement>(
    role: string,
    options?: RoleQueryOptions,
  ) => T;
};

export type AsyncRoleQuery = SyncRoleQuery & {
  findByRole: <T extends HTMLElement = HTMLElement>(
    role: string,
    options?: RoleQueryOptions,
  ) => Promise<T>;
};

export const submitWorkCardQueryContract = {
  addInputButtonName: "Add input",
  dashboardRegionName: "you-agent-factory bento board",
  requestNameFieldName: "Request name",
  requestFieldName: "Text item 1",
  submissionItemsListName: "Submission items",
  submitButtonName: "Submit work",
  submitWorkCardName: "Submit work",
  workTypeFieldName: "Work type",
} as const;

export function getSubmitWorkCard<QueryScope extends SyncRoleQuery>(
  dashboardScope: QueryScope,
): HTMLElement {
  return dashboardScope.getByRole("article", {
    name: submitWorkCardQueryContract.submitWorkCardName,
  });
}

export function findSubmitWorkCard<QueryScope extends AsyncRoleQuery>(
  dashboardScope: QueryScope,
): Promise<HTMLElement> {
  return dashboardScope.findByRole("article", {
    name: submitWorkCardQueryContract.submitWorkCardName,
  });
}

export function getSubmitWorkCardControls<QueryScope extends SyncRoleQuery>(
  submitWorkScope: QueryScope,
): {
  requestName: HTMLInputElement;
  requestText: HTMLTextAreaElement;
  submissionItemsList: HTMLOListElement;
  submitButton: HTMLButtonElement;
  workType: HTMLSelectElement;
} {
  return {
    requestName: submitWorkScope.getByRole<HTMLInputElement>("textbox", {
      name: submitWorkCardQueryContract.requestNameFieldName,
    }),
    requestText: submitWorkScope.getByRole<HTMLTextAreaElement>("textbox", {
      name: submitWorkCardQueryContract.requestFieldName,
    }),
    submissionItemsList: submitWorkScope.getByRole<HTMLOListElement>("list", {
      name: submitWorkCardQueryContract.submissionItemsListName,
    }),
    submitButton: submitWorkScope.getByRole<HTMLButtonElement>("button", {
      name: submitWorkCardQueryContract.submitButtonName,
    }),
    workType: submitWorkScope.getByRole<HTMLSelectElement>("combobox", {
      name: submitWorkCardQueryContract.workTypeFieldName,
    }),
  };
}
