export const factoryValidationAPIErrorMessages = {
  emptyEnvironment:
    "Factory validation is unavailable in this environment.",
  invalidResponse:
    "The factory validation API returned an invalid response.",
  network: "The dashboard could not reach the factory validation API.",
  rejectedRequest: "The factory validation request was rejected.",
} as const;
