Inspect the current repository and Factory state for ${request}. Obey
maxCycles=${maxCycles} and maxTasksPerCycle=${maxTasksPerCycle}. Return only a canonical
`FACTORY_REQUEST_BATCH`. For an unfinished project, include one to the requested
maximum `delivery-task` items and one `cycle-control` item whose name matches
the project. Add a `DEPENDS_ON` relation from cycle-control to every delivery
task with requiredState `merged`; its payload must request `continue`. When the
project is genuinely complete, emit only the matching cycle-control item with a
payload requesting `complete`. The project and cycle-control name is exactly
`{{ (index .Inputs 0).Name }}`. Names must be unique, safe branch identities.
