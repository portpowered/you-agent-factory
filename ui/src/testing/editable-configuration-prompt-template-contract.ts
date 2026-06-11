export const editableConfigurationPromptTemplateContractResponse = {
  availableVariables: [
    {
      category: "ROOT",
      description: "The current work item identifier.",
      example: "{{ .WorkID }}",
      path: ".WorkID",
    },
    {
      category: "INPUT",
      description: "Human-readable name for the first authored input.",
      example: "{{ (index .Inputs 0).Name }}",
      path: ".Inputs[0].Name",
    },
    {
      category: "INPUT",
      description: "Source work identifier for the first authored input.",
      example: "{{ (index .Inputs 0).WorkID }}",
      path: ".Inputs[0].WorkID",
    },
    {
      category: "INPUT",
      description: "Work type identifier for the first authored input.",
      example: "{{ (index .Inputs 0).WorkTypeID }}",
      path: ".Inputs[0].WorkTypeID",
    },
    {
      category: "INPUT",
      description: "Payload for the first authored input.",
      example: "{{ (index .Inputs 0).Payload }}",
      path: ".Inputs[0].Payload",
    },
    {
      category: "MAP_ACCESS",
      description: "Tag metadata for the first authored input.",
      example: '{{ index (index .Inputs 0).Tags "branch" }}',
      path: '.Inputs[0].Tags["KEY"]',
    },
    {
      category: "HISTORY",
      description: "Current attempt number for the first authored input.",
      example: "{{ (index .Inputs 0).History.AttemptNumber }}",
      path: ".Inputs[0].History.AttemptNumber",
    },
    {
      category: "HISTORY",
      description: "Total visit count for the first authored input.",
      example: "{{ (index .Inputs 0).History.TotalVisits }}",
      path: ".Inputs[0].History.TotalVisits",
    },
    {
      category: "HISTORY",
      description: "Failure count for the first authored input.",
      example: "{{ (index .Inputs 0).History.FailureCount }}",
      path: ".Inputs[0].History.FailureCount",
    },
    {
      category: "HISTORY",
      description: "Last error for the first authored input.",
      example: "{{ (index .Inputs 0).History.LastError }}",
      path: ".Inputs[0].History.LastError",
    },
    {
      category: "HISTORY",
      description: "Failure log for the first authored input.",
      example: "{{ (index .Inputs 0).History.FailureLog }}",
      path: ".Inputs[0].History.FailureLog",
    },
    {
      category: "CONTEXT",
      description: "Execution working directory.",
      example: "{{ .Context.WorkDir }}",
      path: ".Context.WorkDir",
    },
    {
      category: "CONTEXT",
      description: "Execution artifact directory.",
      example: "{{ .Context.ArtifactDir }}",
      path: ".Context.ArtifactDir",
    },
    {
      category: "CONTEXT",
      description: "Current project identifier.",
      example: "{{ .Context.Project }}",
      path: ".Context.Project",
    },
    {
      category: "MAP_ACCESS",
      description: "Environment variable access.",
      example: '{{ index .Context.Env "API_KEY" }}',
      path: '.Context.Env["KEY"]',
    },
  ],
  inputCount: 1,
  unavailableAccessPatterns: [
    {
      example: "{{ (index .Inputs 1).Payload }}",
      path: ".Inputs[1].Payload",
      reason: "Only input 0 is available for this workstation.",
    },
  ],
};
