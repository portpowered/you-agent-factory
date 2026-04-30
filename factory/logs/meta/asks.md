> canonical customer-ask surface: edit and prioritize this backlog only in
> `factory/logs/meta/asks.md`. If another path mentions customer asks, treat
> this file as the source of truth.

# customer asks (look to prioritize as necessary, do one thing at a time)

we look generally to make the system amenable to consumption via other customers beyond us. 

## release plans

### go releaser
1. add a goreleaser to the project such that we can configure the project to release to windows/linux/mac on x86 and arm

### CI/CD via github actions
2. add docker ci configurations such that we have mechanisms to deploy the code and confirm that things are worker, then we should update the instructions for our AGENTS.md in our workstations under factory so that the factory submissions are working. We should have a CI step that basically compiels the agent factory and use the agent factory root factory with mock workers and confirm that it passes
- ensure that everything passes


## system deficits

recently, we were trying to test and we had a system outage due to lack of capacity and our system kept retrying without obeying the global resource limits for retries and whatnot for throttles

- the problem i think is that we created this entirely separate abstraction for resource guards that block on overall retries due to throttling failures. 

what i think we should do generally is try to optimize the overall system flow, that is to say, we should look to optimize th overall code to have less abstractions. 

what i mean is we should remove the separate logic for global throttle limts and instead replace it with a global "guard", and add the same type of input guards, but at a higher level. This shouldn't come by default, but be a config that a customer can set at the factory like factory.guards. This guard new one would be called "INFERENCE_THROTTLE_GUARD", and it would be having a InferenceThrottleGuardConfig, that limits on "modelProvider" + an optional "model" as well as a throttle refresh time ("1h" | "2h" |etc ). The transitioner enablements should not have a separate state, but instead should reference the event log for throttle errors and check current clock time for whether this guard is valid.

The logical implementation should be that we flatten this guard doen to the transition guards that we currently have on the petri transition. it should just be treated as any normal guard. The only special thing in our logic is how we do the transformation from the input config into the corresponding itneranal petri transitions/guards.

## quality

- we need to improve our overall system quality, to reduce future rework rates and what not

### website quality

right now the website quality is kind of bad for various reasons, the main ones in consideration for me are as follows: 

1. there is a lack of consistency between the components and we should be using shared components rather than creating bespkoe ones

2. the variables that we use for tailwind are a bit confusing, and use raw variables, whereas we logically want to think in sort of that material design style definition of variables like foreground, bacgrkound,on-foregreoun. We shouldn't use shared classes everywhere because this is strange and we should remove those

3. we need more robust testing, we should look to increase the coverage of our website testing to confirm the system behavior for storybook tests, but we should add a couple more integration tests confirming that our system behaves properly such as for tick controls, and API calls for export PNG and import PNG. 

4. we should have

### backend quality

#### functional tests
The functional tests cover a lot of functionality, but they're a bit obtuse to understand. 
To help with that we should 
- refactor our the fucntional tests into separate packages/folders around what they are responsbile for so that we have less flatness and more structure
- we should also as part of our CI/CD pipelines confirm that the functional tests cover a minimal level of coverage (80%)

#### linting

we don't have much linting automation in place and we should add some
- deadcode
- magic numbers
- basically everything that we would have in golang ci lint

### docs audit
we should audit our docs, some of it is stale and what not, especially the ones we embed as part of our systems cli, we should try to keep those up to date for now

### manual qa
we should try to run a few (20) manual qa runs with mock runners with a variety of possible schemas of factories and feedback into our plans on how things should be fixed. 

### systems quality

we are lacking some copmponents in our docs for systems quality

namely

1. we need a doc that explains our layers of data models
- low level baseline (petri nets/guards/colors/tokens/transitions), the higher level abstractions (workers/work/workstatinos), what is the conflation between the two systems, and why we separate (mostly because the low level is too verbose/complex to expose directly, but that the formalisms provided is nice and clean)

2. we need a doc that defines code quality standards,
- quality is about conformance to simplicity, and we prefer simplicity of the system over making a small seam change. 
- we prefer having systems be easily reasoned about, and look to always have as minimal state as possible. We look to make as much of our systems referentially transparent
- we look to automate quality, because that makes it transparent

