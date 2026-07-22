# deep interface abstractions

The large system definitions are largely complex because they expose an abstraction that is fairly complex. 
We try to create abstractions that are deep and complex. 

## Workers

Workers are things that can do work. 

We can allow many things to work: 

1. An AI Agent wrapped in a CLI can do work
2. A script can do work
3. Some code that we defined can do work
4. A remote webservice API can do work
5. A container running in someone's webhost can do work
6. An AI model running locally can do work. 

## Workstations

Workstations are placed where work is worked on by a worker, and also moved and transformed. 

A workstation can be things like:
1. (inert) do nothing
2. (basic) take in a piece of work and do work on it
3. (mux) take in a piece of work and split it into a hundred pieces
4. (demux) take in multiple pieces of work and generate new work
5. (logical move) just move the work somewher eelse
6. (debounce/timestamp) take a time conditioned step between inputs and debounce and regularize flow
7. (classify) take a piece of work and route it to somewhere based on some condition
8. (consume) consume a work and do nothign with it
9. (event hook) take in events and generate work
10. (cron) run on a clock and repeat every X times
11. (accumulator/skip) listen to x work, drop the first Y, and then let the next N go. 

### More
Generally any operator in a reactive stream is a workstation
https://reactivex.io/documentation/operators.html

## Factory

A factory is a coordinator of work, workers and workstations. 

A factory can be: 
1. a petri graph based transition orchestrator runtime
2. a javascript runtime
3. a stream processor
4. a finite state machine
5. an ad hoc deployment of X agents and a communication protocol

## Automation

An automation is an inert daemon that does some work or is activated by trigger:

An automation can be: 
1. a cron
2. a sse event hook listener
3. a device filewatcher that triggers on file system events
4. a service daemon
5. a device watcher that listens to GPIO or mouse clicks

## Resources/Limits

Limits are things that prevent things from being done: 

1. money
2. request rate per second
3. abstract concurrency limits
4. the number of GPUs you have running
5. time
6. compute resources
7. max active
