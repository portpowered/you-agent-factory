// Package quorum owns the declarative definition of the built-in quorum factory.
package quorum

// BuiltInFactoryJSON is the canonical runnable @you/quorum packaged factory payload.
var BuiltInFactoryJSON = []byte(`{
  "name": "@you/quorum",
  "id": "builtin-quorum",
  "invocationSignature": {
    "parameters": [
      {"name":"input","description":"Text request evaluated by the quorum workers.","required":true,"bindings":[{"kind":"POSITIONAL","position":1},{"kind":"STDIN"}]},
      {"name":"branchProvider","description":"Optional provider for both independent quorum branch workers. When omitted, the operator worker-provider default applies.","externalName":"branch-provider","aliases":["bp"],"bindings":[{"kind":"NAMED"}]},
      {"name":"branchModel","description":"Optional model for both independent quorum branch workers. When omitted, the operator worker-model default applies.","externalName":"branch-model","aliases":["bm"],"bindings":[{"kind":"NAMED"}]},
      {"name":"mergeProvider","description":"Optional provider for the final quorum merge worker. When omitted, the operator worker-provider default applies.","externalName":"merge-provider","aliases":["mp"],"bindings":[{"kind":"NAMED"}]},
      {"name":"mergeModel","description":"Optional model for the final quorum merge worker. When omitted, the operator worker-model default applies.","externalName":"merge-model","aliases":["mm"],"bindings":[{"kind":"NAMED"}]}
    ],
    "examples": [
      {"name":"default-quorum","argv":["Compare the two proposed release plans."]},
      {"name":"role-specific-models","argv":["Compare the two proposed release plans.","--branch-provider","CODEX","--branch-model","gpt-5","--merge-provider","CLAUDE","--merge-model","claude-sonnet-4-20250514"]}
    ]
  },
  "workTypes": [{"name":"task","handlingBehavior":["DEFAULT"],"states":[{"name":"init","type":"INITIAL"},{"name":"branch-a","type":"PROCESSING"},{"name":"branch-b","type":"PROCESSING"},{"name":"merge","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "resources": [],
  "workers": [
    {"name":"quorum-branch-a","type":"AGENT_WORKER","modelProvider":"${branchProvider}","model":"${branchModel}","body":"First independent quorum branch for @you/quorum."},
    {"name":"quorum-branch-b","type":"AGENT_WORKER","modelProvider":"${branchProvider}","model":"${branchModel}","body":"Second independent quorum branch for @you/quorum."},
    {"name":"quorum-merge","type":"AGENT_WORKER","modelProvider":"${mergeProvider}","model":"${mergeModel}","body":"Final merge worker for @you/quorum."}
  ],
  "workstations": [
    {"name":"split-quorum","type":"LOGICAL_MOVE","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"branch-a"}],"onFailure":[{"workType":"task","state":"failed"}],"body":"Prepare the quorum request for branch processing."},
    {"name":"run-quorum-branch-a","type":"AGENT_RUN","worker":"quorum-branch-a","inputs":[{"workType":"task","state":"branch-a"}],"outputs":[{"workType":"task","state":"branch-b"}],"onFailure":[{"workType":"task","state":"failed"}],"body":"Produce branch A's independent assessment of the request.\n\nRequest:\n${input}"},
    {"name":"run-quorum-branch-b","type":"AGENT_RUN","worker":"quorum-branch-b","inputs":[{"workType":"task","state":"branch-b"}],"outputs":[{"workType":"task","state":"merge"}],"onFailure":[{"workType":"task","state":"failed"}],"body":"Produce branch B's independent assessment of the request.\n\nRequest:\n${input}"},
    {"name":"merge-quorum","type":"AGENT_RUN","worker":"quorum-merge","inputs":[{"workType":"task","state":"merge"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}],"body":"Synthesize the quorum assessments into one final response.\n\nOriginal request:\n${input}"}
  ]
}`)
