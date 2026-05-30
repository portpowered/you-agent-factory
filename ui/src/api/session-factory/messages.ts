export const sessionFactoryAPIErrorMessages = {
  invalidResponse: "The session factory API returned an invalid response.",
  network: "The dashboard could not reach the session factory API.",
  rejectedRequest: "The session factory API rejected the request.",
  rejectedSaveRequest: "The session factory API rejected the save request.",
  unavailableInEnvironment:
    "Session factory editing is unavailable in this environment.",
} as const;
