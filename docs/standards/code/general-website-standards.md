# overview

this provides a general structure of how websites should be structured and the general north start for which we look to restructure the website interfaces. 

We define this generally to be applicable to any website that is usable by anyone

## Componentization

- system must define itself by using a shared design system
- all structures should be constructed from components, granular to the level of buttons, headers, etc that are reusable

layers should be defined as follows: 

## Testing

### overall 
Testing we define to cover 5 layers

unit tests :- covers the general functionally for a class/function, usually jest style tests
component tests :- covers the general functionality of a component, is run using storybook runners, with , test mocks
functinal tests :- covers the entire website, with a mocked backend via mock service workers, usually via playwright and others. 
integration tests :- full website integration with backend
performance tests :- test the website behavior at load, duration for leaks, etc. 
### best practices

we recommend to have lighter levels of unit tests/perforamnce/integration tests. 
We recommend but have majority of testing beh in the component tests, and functional tests. 

### checklist coverage
- have automation CI/CD target testing at 80/90% functional test coverage
- have automation CI/CD cover unit, integration, performance, functional tests and ensure they pass
- have functional tests cover the functionality of the website
- have integration tests available to confirm regressions of website
- have performance tests validate system behavior at load (high events, high data load)


