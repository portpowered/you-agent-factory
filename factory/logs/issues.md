graph-edito issues
- text for the toolbar is getting cut off on the bottom, so we need to allow it to overflow on theg raph-viewport. 

- the current edge editor and the observe mode has two modellings of edges we should use semantic edge ids, rather than default ones, remove all the changes that made them non semantic. 

- the current edge graph is incorrectly rendering because it doesn't error out properly. 
- the current edge graph should be configured with an onError edge handler that precludes it from running and crashes the program if there is an error.  

- make the handle nodes just simple dots with outline colors. 
- make the graph editor draft state transformation from the snapshot topology. make the transformation extremely thin. all it should be doing is attaching context to the topology such as edge removals and node connections and new nodes and other context metadata. 
- stuff that is contextually stored or should be stored in the topology should be stored in the topology. 

- update the grpah topology rendering to use the workstation topologies from the draft state regardless of if its in edit/observe mode

- update the graph topology to have tests for rendering temporary workstations/temporary work states. 

- hide the delete button until we figure out how to get it working. 

- hm... okay so how can we test this more easily, we want it so that given a given graph, when i call the hook that manipulates the graph draft workstations then the graph should render a new node. 

we should model the topology state to be a standardized graph. 
- the graph shouold be modelled as a bespoke state service. 
- the service should expose hooks. 
- 

### bespoke problems. 
- make the monaco editor expandable, do the standard editor squiggly undlerine on failing lines
- refactory the functional tests since they're getting fairly large. 
- bin/you default CLI command should support --v, to add verbose logs
- bin/you CLI docs should add command support for `docs mock-workers`, `docs replay-record`
- we should configure the backend functional tests to have a maximum of 10 lines per functional test. the runtime_api tests should be broken down along the lines of feature sets
-- hosted_models
--- config
--- api
--- cli
--- event stream

-- config
--- cli
--- config

-- sessions
-- server/dashboard
-- work//submission
-- workstations/poller
-- event_stream

-etc. 

## test coverage out

- we should have the functional tests and the corresponding unit tests output to github CI summary the total coverage of the unit tests, and the total coverage of the functional tests separately. Also at a per package level on the pkg directory. 

## bundled files makefile removal
- don't include the projecct makefile as part of the bundledf iles


## bundled file validations
- [bundled-file-content-inline] resourceManifest.bundledFiles[0].content.inline: missing required 'inline' field

-- this error is insufficientn, we should add the explicit filename/file that this is referencing. 
-- induced by empty makefile... we should remove makefiles from the references unless explicitly referenced. 

## factory logs
agent-factory logs should be separated by date/time same as we did for the recordings


## factory input validation messages
:"~default","folder_path":"factory","factory_dir":"factory","error":"net validation failed: TYPE_COUNT_COLLISION - transition \"process\" has a type-count collision on output arcs for work type \"task\": 1 consumed input(s) but 2 routed arc(s) (at transition:process.output_arcs:task)"
--> this shoudl be exported as an ERROR with said message, rather than silently failing. 

-- the website crashes sometimes if you modify the graph? why? 
-- not related to network traffic seemingly. 

-- website editor text has a bunch of notifications on failure

-- website says that it is awaiting the event stream but its not ever clearing the error? 

data: {"context":{"eventTime":"2026-05-28T06:30:14.722119Z","sequence":0,"tick":0},"id":"factory-event/run-started","payload":{"factory":{"name":"complete","resources":[{"capacity":10,"name":"executor-slot"}],"workTypes":[{"name":"review","states":[{"name":"init","type":"INITIAL"},{"name":"in-review","type":"PROCESSING"},{"name":"to-complete","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]},{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"in-review","type":"PROCESSING"},{"name":"to-complete","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"executorProvider":"SCRIPT_WRAP","modelProvider":"CODEX","name":"processor","type":"MODEL_WORKER"}],"workstations":[{"behavior":"REPEATER","id":"process","inputs":[{"state":"init","workType":"task"},{"state":"available","workType":"executor-slot"}],"name":"process","onContinue":[{"state":"init","workType":"task"}],"onFailure":[{"state":"failed","workType":"task"}],"onRejection":[{"state":"init","workType":"task"}],"outputs":[{"state":"in-review","workType":"task"},{"state":"available","workType":"executor-slot"}],"type":"MODEL_WORKSTATION","worker":"processor"},{"id":"review","inputs":[{"state":"in-review","workType":"task"},{"state":"available","workType":"executor-slot"}],"name":"review","onFailure":[{"state":"failed","workType":"task"}],"onRejection":[{"state":"init","workType":"task"}],"outputs":[{"state":"to-complete","workType":"task"},{"state":"available","workType":"executor-slot"}],"type":"MODEL_WORKSTATION","worker":"processor"}]},"recordedAt":"2026-05-28T06:30:14.722119Z"},"schemaVersion":"agent-factory.event.v1","type":"RUN_REQUEST"}

data: {"context":{"eventTime":"2026-05-28T06:30:14.722155Z","sequence":1,"tick":0},"id":"factory-event/initial-structure/0","payload":{"factory":{"name":"complete","resources":[{"capacity":10,"name":"executor-slot"}],"workTypes":[{"name":"review","states":[{"name":"init","type":"INITIAL"},{"name":"in-review","type":"PROCESSING"},{"name":"to-complete","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]},{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"in-review","type":"PROCESSING"},{"name":"to-complete","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"executorProvider":"SCRIPT_WRAP","modelProvider":"CODEX","name":"processor","type":"MODEL_WORKER"}],"workstations":[{"behavior":"REPEATER","id":"process","inputs":[{"state":"init","workType":"task"},{"state":"available","workType":"executor-slot"}],"name":"process","onContinue":[{"state":"init","workType":"task"}],"onFailure":[{"state":"failed","workType":"task"}],"onRejection":[{"state":"init","workType":"task"}],"outputs":[{"state":"in-review","workType":"task"},{"state":"available","workType":"executor-slot"}],"type":"MODEL_WORKSTATION","worker":"processor"},{"id":"review","inputs":[{"state":"in-review","workType":"task"},{"state":"available","workType":"executor-slot"}],"name":"review","onFailure":[{"state":"failed","workType":"task"}],"onRejection":[{"state":"init","workType":"task"}],"outputs":[{"state":"to-complete","workType":"task"},{"state":"available","workType":"executor-slot"}],"type":"MODEL_WORKSTATION","worker":"processor"}]}},"schemaVersion":"agent-factory.event.v1","type":"INITIAL_STRUCTURE_REQUEST"}

data: {"context":{"eventTime":"2026-05-28T06:30:14.728969Z","sequence":2,"tick":0},"id":"factory-event/factory-state-change/0/RUNNING","payload":{"previousState":"IDLE","reason":"run started","state":"RUNNING"},"schemaVersion":"agent-factory.event.v1","type":"FACTORY_STATE_RESPONSE"}

data: {"context":{"eventTime":"2026-05-28T06:32:01.636444Z","sequence":3,"tick":2},"id":"factory-event/factory-change/2","payload":{"factory":{"name":"UNDEFINED","resources":[{"capacity":10,"name":"executor-slot"}],"workTypes":[{"name":"review","states":[{"name":"init","type":"INITIAL"},{"name":"in-review","type":"PROCESSING"},{"name":"to-complete","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]},{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"in-review","type":"PROCESSING"},{"name":"to-complete","type":"PROCESSING"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"executorProvider":"SCRIPT_WRAP","modelProvider":"CODEX","name":"processor","type":"MODEL_WORKER"}],"workstations":[{"behavior":"REPEATER","id":"process","inputs":[{"state":"init","workType":"task"},{"state":"available","workType":"executor-slot"}],"name":"process","onContinue":[{"state":"init","workType":"task"}],"onFailure":[{"state":"failed","workType":"task"}],"onRejection":[{"state":"init","workType":"task"}],"outputs":[{"state":"in-review","workType":"task"},{"state":"available","workType":"executor-slot"}],"type":"MODEL_WORKSTATION","worker":"processor"},{"id":"review","inputs":[{"state":"in-review","workType":"task"},{"state":"available","workType":"executor-slot"}],"name":"review","onFailure":[{"state":"failed","workType":"task"}],"onRejection":[{"state":"init","workType":"task"}],"outputs":[{"state":"to-complete","workType":"task"},{"state":"init","workType":"review"},{"state":"available","workType":"executor-slot"}],"type":"MODEL_WORKSTATION","worker":"processor"}]}},"schemaVersion":"agent-factory.event.v1","type":"FACTORY_CHANGE"}

data: {"context":{"eventTime":"2026-05-28T06:32:01.636742Z","sequence":4,"tick":1},"id":"factory-event/factory-state-change/1/COMPLETED","payload":{"previousState":"RUNNING","reason":"run stopped","state":"COMPLETED"},"schemaVersion":"agent-factory.event.v1","type":"FACTORY_STATE_RESPONSE"}

data: {"context":{"eventTime":"2026-05-28T06:32:01.636759Z","sequence":5,"tick":1},"id":"factory-event/run-finished","payload":{"state":"COMPLETED","wallClock":{"finishedAt":"2026-05-28T06:32:01.636759Z","startedAt":"2026-05-28T06:30:14.722119Z"}},"schemaVersion":"agent-factory.event.v1","type":"RUN_RESPONSE"}


-> todo fix bugThe draft has been cleared and the graph is waiting for the latest factory-change event refresh.

## factory validate API

when we make the factory modifications, the website should validate the current payload to a factory validate API that checks where the errors are, similar to how the parser is done as part of the AST. 

- the website should notify of the induced error when the input is invalid. 

## CLI session list

the CLI should be able to perform the set of session operations

```
you session list
you session delete <session-id>
you session create --factory-directory <session-directory>
```

use the enumeration control patterns we use for work

## CLI factory save

we shoudl enable customers to pass in a flattened factory json and save it to a directory so that the CLI can test behavior. 

```
you factory create --file <my-factory-file> --name <factory-name> | default to default factory if no name is set. --session-id <id-of-session-to-update> , if set we look to update the factory of the session rather than the current folder. 

you factory delete --name | name is required , ~default is the default factory
```

these should update the factory

## named directory/absolute path of current factory. (DONE)

right now the default factory path is just the relative factory directory, not the absolute, we should chang ethat and make it the absolute path and add corresponding tests. 

## standardize bento card headers (DONE)

we shoudl make all the bento cards use the same header component that the cards can add additional controls to, that way we can make sweeping changes easier. 

we should make additionally a useQuery media query that shrinks the buttons shape if the view port is a mobile view. 

## workstation editor components

right now the workstation supports certain fields, but not all of them
- it does not let you set the workstation type
- it does not let you set the workstation editor
- it does not let you set the name of the workstation
- it does not let you set the workstationlimits, guards, worktree, workingdirectory, env, etc. 

- the workstation editor from the current selection is not modifying the same viewmodel as the corresponding state modifier from the activity grpah

update the workstation current selection to modify and save using the same viewmodel state that the factory graph is modifying and add the new configurations. 

add tests that validate the behavior works to write against the service API with appropriate configurations. 

## factory init issues (WIP)
factory init generates a factory that is invalid due to reference to non-existent resource agent slot
- run validation as part of functional tests of the init factory configuration, so that this doesn't happen again by running the factory validate endpoint against the initial factory.json. convert the init factory current system to use a single factory.json schema rather than strewn out. 
- 


## bug

Error: factory run: submission hook "external-submit" generated batches: work_request: works[0] ("factory-tab-website-init") has invalid content/payload: payload conflicts with explicit content

## factory import (multi-session factory import doesn't work) (WIP)

we want customers to be able to import factories on any tab in the factory. right now it only works on the first/default tab. 

This is because the submission for import goes to the /factories endpoint, rather than the factory-session specific factory endpoint. 

on the second tab, the factory submission on import targets the default factory's factory directory, source directory, etc. 
- please update the import button/functionality to be session context aware, such that it derives its context for importation from the current-session that is active and submits to the appropriate factory-session/{session-id}/factory to update the running factory for a session and correspondingly creates it. 
- please add functional tests that validate the payload is correct and the correct endpoints are called when running importation on a factory tab. 

- ensure the backend works as expected/frontend interaction works as expected by adding itests that exercise the new tab/import path.

## autocomplete suggestor (WIP)

- the autcomplete is trying to suggest even when outside of a {{ }} block, let's not do that. 

## save current selection (WIP)

right now the current selection button is not clickable even after the prompt contents are modified to submit a change. 

confirm this by adding a website test that renders the component and attempts to save the prompt. it should fail. 

please confirm this is true then fix it. 

additionally, we want the save to be more prominent. when the save is available, can we make it yellow instead? 

### autocomplete contents (WIP)

the autocomplete contents denoting which variables are avilablle/not available is useful but too verbose. can we encapsulate that block in an expandable. 

## workstation logic mismatch (WIP)

- right now when the connecdtion is established in factory editing, it is blocked if for example "one worker assignment" must be kept. 
- let's not block operations and just allow them generally when we're performing factory edits. 
- also the logic is wrong for LOGICAL_MOVE style workstations where no worker is needed. 
- we should update logical move workstations to not expose a worker handle. additionally, we should update the validation rules to reflect this difference on the UI. 

## mock-workers-docs-script-workers

-- the docs don't explain how to make script workers for mock workers work. should add more details

## mock-workers-mix-with-real

-- sometimes you want mostly mock workers, but also partially real workers to test hybrid behaviors. 
-- extend mock workers with a mock/real mix functionality. 
-- add tests validating the expected behavior as a functional test
-- the real workers have to be specific configurations on the mock-workers json such that they are explicitly overriden to be non mock, we don't want to change default beavior. 
