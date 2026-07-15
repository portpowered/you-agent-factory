# problem statement

right now the test isn't sufficiently tested even though its large leading to systems degradation

# Solution 

we add functional tests for everything and move the service interfaces closer to the user facing interaction layers: 

1. customers interact via the API interfaces or the CLI interfaces for controlling the objects. 
1.1. the injection of runtime daemons and what not is done as part of the injection runtime and configuration runtime as part of service instantiation
2. customers do not directly call into the service constructors
2.1. there is no internal calls to constructors
3. we have validation and test mechanisms to confirm that systems are truly tested e2e from the entrypoints. 
3.1. we use package level coverage requirements based solely on functional tests
4. we inject daemons/mocks at the edge level to simulate full system functional tests as much as possible
4.1. for pollers/sse from external systems we mock the daemons
4.2. for external APIs we mock the external APIs
4.3. for external CLIs we mock the process.exec layer. 

# comopnents needing testing

1. api layers
1.1. mcp
1.2. restful interfaces
1.3. sse streams
1.4. cli

2. functionality
2.1. dynamic workflows
2.2. system runtimes
2.3. custom daemons
2.4. pollers/linear invocations
