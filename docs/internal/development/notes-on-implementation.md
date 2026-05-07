# Notes on the agent factory. 

The implementation that was one shot by the LLM sort of works, but it doesn't have proper architecturing: 

- There should be a service layer, that is responsible for orchestration file reads, config, and managing the factory running inside. 
- There should be a factory abstraction layer, which is responsible for the logic abstraction
- There should finally be an internal logic layer (aka the petrinet layer), which is responsible for the delegation of work, measuring work orchestration, and validating

Overall, when the pieces are in concert, then everything works okay. 

The problem with the implementation is as follows: 

## Abstraction leak 

- there is too much leakage between the factory and the internal logic layer. The factory layer should work solely on the abstractions level. The internal logic layer, should only handle the basic abstractions. 
-- i.e. we have this concept of resources, but that shouldn't be a problem that the petrinet layer should care about. It should just be modelled as tokens in places. And the transitions should consume tokens, and operate on said tokens. 
-- when the tokens are consumed by the engine, then the engine can move to the next step. 
-- the problem is that the concepts are too intermingled. for example, the petri and the net packages share the same layers of logic, but for some reason or another the net layer contains abstraction limits and constraints. 
-- those should not be really an abstract that the net abstraction should deal with, it should be at the factory level. There needs to exist a factory state level, so we should rename the net package to the factory/state package. 
-- then the factory/state package should make a reference to the petri net package

## Configuration and UX
- the interfaces that are presented to the consumer are uncomfortable. We would rather the consumer take in a basic config file, and have that config file be used as the abstraction for defining a factory. 


## Too much logic on the interface layer
The CLI and pkg/api layer are responsible for constructing the factory, and all the business logic involved in that. That's bad because the separation makes the use case insufficient. 
We recommend that the root CLI layer should only be generic Cobra command execution. There should be command-specific pkg/cli packages that reference the different functions. 
We should NOT use our own bespoke flag mechanism, we should do something similar to what was done for the CLI. 

Then finally the pkg/cli interface should call into the service layer to instantiate the factory with all the appropriate configurations. The factoryservice should take a struct that is roughly commensurate to the input of the factory CLI. 

## Functional tests
we should be having better tests that test based on the config rather than rely on the fluent builder to build internal construct. 

rather, we want to test the mapping from a config input that the customer lays into us, and handle that, rather than deal with the simpler logic of only doing the fluent builder. 


## Weird abstractions on the petri layer
### dependencies and relations
When a token should be dependent on another token, there is no test for this currently. 
We need to implement a test. 
The logic should go as follows: 
- we add a new token color, which denotes the list of other tokens that needs to be done, before the token can be done. 
- when the token is being converted from the work request, we ensure that the token annotation is defined referring to the ids of the dependent tokens and their necessary state for completion. 
- when transitioning from the original state to the next state, the guard should check the colors and the world state, and confirm that the world state does indeed have the tokens in a completed state. 
- we should similarly denote, the child as cascadingly failed if the dependency token failed already and transition the world to fail state collapse, and terminate rather than leave the token hanging forever. 

### cascading failure
- correspondingly, we should update the guards to consume the world state rather than be agnostic to it, as they can't make reasonable decisions otherwise.
- what this likely means is that by default we should have this sort of default transition that consumes any type of token, that guards against whether the parent has failed. It ticks, such that if the parent has failed, then it transitions the children into a failed state. 

### guards and dynamic fanout
Right now there's a dynamic fanout test that is testing the petri layer is able to handle multiple fanout. This implementation is wrong, because it presumes that we know how many elements are going to come in at an input time. 
Rather than doing that, we need to have the petri net mapper generate a guard token in a group place, and have the transition consume the guard token and the place tokens only if the number of place tokens and guard tokens count match. 
Instead, its doing a weird fixed thing which doesn't make sense. 

## lack of metrics and visibility
If i run the factory right now, its kind of a pain in the ass to see what's going on, as we only print the markings of things. 
First, the markings and printing to the command line should be something that is done on the service level. 
Second, the function right now only records token markings, which (while it works), makes it hard for us to reason about the system and the bottlenecks. 
We need a new subsystem that is recordnig the times of items in the qeueu, as well as recording and presenting (which resources are limiting us): 
why did the the items fail? i.e. too many retries, etc. 

## integration tests: 
- originally the tests were written with integration tests that validated that the system worked by setting up the entire configuration data harness. This was terrible for anyone that wanted to know. It leaked too much information for hte user. 
- intead, what we want is that the integration tests validate the behavior fo the system when we manipulate the factory/factory.json file. 
- we want to confirm that the execution does what we expect it to. 
