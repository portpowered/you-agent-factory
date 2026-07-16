// Package classifier defines the @you/classifier packaged factory.
package classifier

// BuiltInFactoryJSON is the canonical runnable @you/classifier packaged factory payload.
var BuiltInFactoryJSON = []byte(`{
  "name": "@you/classifier",
  "id": "builtin-classifier",
  "workTypes": [
    {
      "name": "task",
      "handlingBehavior": ["DEFAULT"],
      "states": [
        {"name": "init", "type": "INITIAL"},
        {"name": "small", "type": "PROCESSING"},
        {"name": "medium", "type": "PROCESSING"},
        {"name": "large", "type": "PROCESSING"},
        {"name": "complete", "type": "TERMINAL"},
        {"name": "failed", "type": "FAILED"}
      ]
    }
  ],
  "resources": [],
  "workers": [
    {
      "name": "classify-complexity",
      "type": "AGENT_WORKER",
      "preset": "small",
      "body": "Classify the request complexity. Reply with exactly one plain label: small, medium, or large."
    },
    {
      "name": "run-small",
      "type": "AGENT_WORKER",
      "preset": "small",
      "body": "Complete the request directly and concisely."
    },
    {
      "name": "run-medium",
      "type": "AGENT_WORKER",
      "preset": "medium",
      "body": "Complete the request with the appropriate analysis and detail."
    },
    {
      "name": "run-large",
      "type": "AGENT_WORKER",
      "preset": "large",
      "body": "Complete the complex request carefully and thoroughly."
    }
  ],
  "workstations": [
    {
      "name": "classify-complexity",
      "type": "CLASSIFIER_WORKSTATION",
      "worker": "classify-complexity",
      "inputs": [{"workType": "task", "state": "init"}],
      "classificationRoutes": [
        {"label": "small", "outputs": [{"workType": "task", "state": "small"}]},
        {"label": "medium", "outputs": [{"workType": "task", "state": "medium"}]},
        {"label": "large", "outputs": [{"workType": "task", "state": "large"}]}
      ],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "body": "Classify this request by complexity and return only small, medium, or large."
    },
    {
      "name": "run-small",
      "type": "AGENT_RUN",
      "worker": "run-small",
      "inputs": [{"workType": "task", "state": "small"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "body": "Handle the small-complexity request."
    },
    {
      "name": "run-medium",
      "type": "AGENT_RUN",
      "worker": "run-medium",
      "inputs": [{"workType": "task", "state": "medium"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "body": "Handle the medium-complexity request."
    },
    {
      "name": "run-large",
      "type": "AGENT_RUN",
      "worker": "run-large",
      "inputs": [{"workType": "task", "state": "large"}],
      "outputs": [{"workType": "task", "state": "complete"}],
      "onFailure": [{"workType": "task", "state": "failed"}],
      "body": "Handle the large-complexity request."
    }
  ]
}`)
