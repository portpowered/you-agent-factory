Generally, the impetus for writing this is i wanted a lightweight orchestrator that could be used for whatever workflow i wanted. 
To that end, the existing systems were too heavy, or too opinionated on their flows. 

With __infinite you__, you just run a binary, you can check in the workflow/AGENT files and that's it.


| Program          | Recursion, FanIn, Stateful | Custom workflows | Agent harness support   | Just a file   | Durable workflows | Relatively stable |
| ---------------- |:--------------------------:|:----------------:|:-----------------------:|:-------------:|:-----------------:|:-----------------:|
| Infinite you     |             X              |           X      |           x             |      x        |                   |         X         |
| Random Scripts   |             X              |           X      |           x             |      x        |                   |                   |
| Gas Town         |             X              |                  |           x             |               |                   |         X         |
| DBOS             |             X              |           X      |                         |               |         X         |         X         |
| Dagster          |                            |           X      |                         |               |                   |         X         |
| N8N              |             X              |           X      |                         |               |                   |         X         |
| Temporal         |             X              |           X      |                         |               |         X         |         X         |

### Custom scripts

You can just write custom scripts with python, bash/powershell:
- ralph loop
- auto researcher

I did the same thing, but needed to run the system on my windows and mac laptop and also the thing kept failing as i added more complex stuff to it. 

### [Gas town](https://github.com/gastownhall/gastown)

This is an alternative agent orchestration framework. It works quite well but its rather opinionated on how it does stuff 
- reliance on doltDB,beads
- rigid workflow structure. 
- git 

With __infinite you__, there's no fixed structure so you can do whatever you want with it. i.e. if you want the system to not submit anything and just spawn thirty QA bots reviewers to ensure the code conforms to your standards and is passing all the tests you can do that. 

### Dagster
This is a standard workflow engine, it works okay, but there's no affordances for agent harnesses so you have to write one yourself. 
Also its a directed acyclic graph, but work processes are never really DAGs, they're more like spaghetti. In the end I couldn't figure out how to make it do a standard execute (loop) -> review loop. 

### DBOS
This is a complex durable workflow engine. Its fairly lightweight relative to its alternatives. 

As a comparative, __infinite you__ doesn't have mechanisms for transactional consistency or durability of execution. 

But its still too heavy, since i didn't want to write code. The code vs config thing is a tradeoff, code's too hard to grok quickly frankly, config is verbose but any config with verbosity theoretically induces itself to levels of complexity wherein you end up mapping to code anyways. Unless you're SQL or something. 


### Temporal 
This is also a complex durable workflow engine. 
This is flexible enough to do what I wanted. 
This thing is just way too heavy for the use case of just having an AGENT run, relative to DBOS and alternatives. 

### N8N
This is a robotic process automation tool. It has theoretically what i want, but it generally confused me. Its too heavy. I couldn't really get it to do what i want. 
