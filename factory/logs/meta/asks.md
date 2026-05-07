> canonical customer-ask surface: edit and prioritize this backlog only in
> `factory/logs/meta/asks.md`. If another path mentions customer asks, treat
> this file as the source of truth.

# customer asks (look to prioritize as necessary, do 3/4 things at a time)

we look generally to make the system amenable to consumption via other customers beyond us. 

You are to run autonomously until i don't want you to Which is may 25th, 2026. Afterwards, please shut down any instances of infinite-you, and terminate. 

## quality (P0)

### global checklists
we defined in here a variety of checklists that define how to confirm things work,

- please follow the checklists for websites, backend and check our systems for general conformance and create tasks that move the system in alignment. i.e. if the checklists denote we should enable wcag, enable localization, enable perf tests as part of CI, we should go about setting up as part of our meta theory of the world how conformant of the standards we are and move towards converging to them. 

https://github.com/portpowered/checklists/blob/main/website-development-checklist.md
https://github.com/portpowered/checklists/blob/main/backend-development-checklist.md
and any other new checklists, you should look up the package to see if any new ones appear every day or so. 


#### backend/website testing

We need better quality tests coverage. 
1. please ensure that the fucntional tests cover at least 90% of all non generated code in the pkg directory. 

We need better website test coverage
1. please ensure that the tests for the website cover at least 90% of all non generated code in the ui/src directory. 

#### Code quality

We look to reduce the complexity of the system and reduce the overall code base: 
1. look at pieces in the backend or the website in which we can remove or simplify the logic. 
2. the general idea is we want to conform to the overall data model while shrinking the overall system that we need to maintain





