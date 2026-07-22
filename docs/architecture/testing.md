
# Testing

The archtiecture for the you agent factory is layered as a series of abstractions. 

## Test pyramid

1. Unit tests :- tests the file in isolation
2. Functional tests :- tests the entire component in isolation (the backend with mocked dependencies)
3. Integration tests :- tests the entire component with full integration with other components (frontend with backend + some mocks at the edges)
4. Load/Stress Tests :- tests the entire component with some high level of input (along with integration with other components)

We prefer the priority to be

functional tests > unit tests > integration tests > load tests. 

functional tests give the best bang for your buck, and unit tests are second. 
Integration test and load tests are needed but tend to break things so we keep them as light as possible. 

## partially disavowed

subcomponent tests :- these are tests that say instantiate a service within the backend to tests its interactions. 
-- we prefer to tests full flow with mocks,as internal contracts are never stable in the backend. 
-- we have these in the frontend as a stable tradeoff, as they test a rough contract. i.e. a component level test.


## Test structures

### Functional tests

Fucntional tests are tests that interact with a component entirely with abstracted external components. 
i.e. testing the backend with mocked CLI interactions for the harnesses for AI agents, but instantiating the whole thing. 

#### structure
We have functional tests located in tests/functional. 

the structure is roughly
```
tests/functional
    
    <cross-interaction-gate-interactions-are-also-rooted>
    /features
        /dynamic-workflows
            <sub-abstractions>
            /cli
            /rest
            /mcp
            /cross-function
        /transports <- test baseline transports work
        /help-info
        /multi-session
        /petri-graphs
        /packaged-factories
        /system-configw
        /throttling
        ...etc
```

#### interaction behavior
These tests work roughly as if they are interacting with the CLI/API directly. 
i.e. 

test -calls into -> the pkg/root.go with some mocked parameters and then instantiates the server instnace. 

tests that are functional tests never interact directly with the services inside the backend or the frontend. 

#### mock
during this time, the tests injects such things as: 
1. mock worker runners that abstract os.process execution so that the CLI integrations are recorded for verification in the run
2. mock http servers that the APIs can interact with such as the linear API or Jira APIs. 
3. mock http instance so that its cheaper for the server to run
4. mock configuration and runtime so that the appropriate runs are done. 



