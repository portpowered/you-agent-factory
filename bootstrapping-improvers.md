# problem

we have a variety of packaged factories that are now merged, but we don't yet know if this will actually help or that they work properly. 

To validate we recommend to run a series of experiments, wherein the packaged factories bootstrap themselves against the actual you-agent-factory repository. 
we have a bunch of service restructuring work that remains under C:\Users\andre\work\portos\infinite-you\docs\internal\packaged-service-structure, and those are a good fit as an experiment. 

## overall plan
- for each of the high level factories attempt to create a subworktree wherein we can execute a specific packaged-factory
- validate the outputs of each subfactory
- measure the target outputs (did it do what we expected, did it achieve a suitable outocme)
- if it didn't iterate back on the factory schema and reloop
- for each of the experiments write down a experiment denoting what was the target/expectation/result/timestamp, explicit command that was run. - target changes that were used/which factory any prompt changes made. 
- commit on every system change. 

## recommended models
we recommend that these models are used and those providers are used
### codex
gpt-5.6-sol-medium <- use for large intelligence
### cursor-acp
cursor-grok-4.5-high - Cursor Grok 4.5 <- use for large intelligence
composer-2.5 - Composer 2.5 <- use for small itnelligence

## example

target: we want to validate that that test refactoring works well so we use 
expectation: the models execute the appropriate plan output, and the target is running
worktree: <my-worktree>
result: <To be filled in after results>
timestamp: 7/30/2026 4:45 AM PST
explicit command run: `you run -a "@you/plan-parallel" --plan-provider codex --executor-provider cursor-acp --executor-model blah
commit hash of the factory: <blah>