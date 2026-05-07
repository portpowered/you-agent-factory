# The Zen of Flow

## Problem Statement

Problematically, people who manage workers unknowningly manage them wrong, leading to deleterious outcomes.

### Synopsis 
As AI agents are getting better, more people will have to manage. These "AI Agent managers" are given the responsibility of governing work, yet have not yet had the experience or education to do so. 

This doc explains how the nature of the management of work, AKA flow. The intent is this information will help "AI agent managers" to achieve outcomes.

Overall:

Work is a flow of inputs and outputs. The flow is built to achieve outcomes. Optimize for outcomes. The flow has a defined behavior constrained by its nature. Optimize flow to improve outcomes. Optimize systematically, with visibility, and continuously.

## Note: 
We recommend to experiment with business workflows and processes to see outcomes before reading the rest of the paper after this section.
Prior to having experience, most everything will be oblique to you.

Try the following first:
- Define the goal of your work. Or the aggregate company that governs your work. How do you measure if its successful?
- Define a successful project. Is it useful if the project meets a target number/SLA? 
- Think about why the shopping line is taking so long? Would adding more lines help? Would more tellers?

More concretely:
- What happens when you add checklists to your PRs? Does that make the quality better? 
- What if you define acceptance criteria to your stories? Does it meet the expected outcomes? 
- What if you encode templates on how docs should be structured? Does that reduce the time to getting an output? 
- What if you give agents memory? Does it provide you with your target outcomes more often? How would you know? 
- What if you tell the agents to tell you what to do? Do the suggestions help improve goals? 


# Flow 
Below are ways to think about flow as you build flow.
We outline how to think about flow, some characteristics of flow, and some methods of optimizing flow. 

---

## I. Thinking about flow
### Systems thinking

Work is about aggregate outcomes, not processes.

Do not look at individual components and optimize, look at the overall flow and optimize. Looking at individual components leads to harmful outcomes to overall flow.

If you've defined a measure, does it matter? If it does not help the aggregate flow, no. The measure is not the flow. The flow is the outcome. 

### Incentives

Incentives govern how work is done. 

The outcomes of processes consequent of incentives can be good or bad, unintuitive to your expectation. Always measure consequences. 

Do not put concrete goals, they become bad incentives. Waste fills the aggregate leeway you give it. Instead define flows and measure. 

### Complexity

Largeness necessarily induces complex and consequently incomprehensible. 

Abstract complexity to reduce mental burden, at tradeoff of precision and correctness. Work less. Do less. Less fails less. Work fails. Remove the work.

---

## II. Characteristics of Flow

### Chaos

Things will fail, and things will happen randomly within some distribution.

Non deterministic flows have inherent variance. Handle tail failure and probability distribution. You may use process controls the variance output, but processes do not remove variance.

Do not attempt to reduce variance to 0. This is non feasible. Do attempt to reduce aggregate variance. 

### Bottlenecks

The rate of change in a flow is bounded by the throughput of its slowest element.

It does not matter how fast you write code if your deployment pipelines take two months. More work in progress only means more rework. When agents work for you, do not shove in more if they are all waiting to deploy. Optimize the bottleneck first; everything else is noise.

### Backpressure

Work throughput is constrained by aggregate work in the flow. 

When you push, the flow overflows. Bottlenecks deepen so that nothing completes. Let your flows pull work from you. Let processes break requests into smaller asks. Each element in the flow pulls from the one before it. 

### Optimal size

Work input has an optimal size. 

The right amount of work is found by simulation and modelling: identify the bottleneck, measure its throughput, and size concurrency to match. Adding hands to a task that cannot be divided only adds confusion. Model the flow before you scale it.

### Compounding

The cost of fixing something grows the later you find it.

Put more guards on the input of the flow. validate the PRD before an agent writes a line of code. An agent that catches a design flaw in the requirements saves ten agents fixing code downstream. Design review catches what code review misses. Code review catches what QA misses. Move the catching upstream, as close to the source as possible.

---

## III. Optimizing flow

### Make things visible

Lack of goal means no outcomes.

Present to yourself what the aggregate flow looks like. Define the flow as such. Its only when you know the aggregate flow can you attempt to modify and optimize it. 

Set up indicators you can follow: fault rates, bug rates, canaries, load tests. Revisit them. Ensure they remain useful. Measure your goals against what is visible. Remove measures randomly and perform qualitative analysis along with quantitative. Use countermeasure metrics. 

### Build determinism over stochasticity

Make errors impossible, or immediately obvious. 

Use scripts when possible instead of agents. If you must use agents, make the process as simple and clear as possible. When a defect is detected, the flow stops. It does not continue. Make things blow up visibly when they fail. Silent failure is the enemy of flow.

### Standardize the work

Systematic process is worth more than gut feeling.

Better models do not guarantee better code. Encode checklists, standards, and templates into your system. A process that lives in someone's head dies when they walk away.

### See with your own eyes

Reports are shadows on the wall. 

The only way to know is to go and see. Give agents autonomy. Give them memory. Let them surface what they find. But also do the work yourself, regularly. If you cannot walk the path, you cannot design the path.

### Model the system

Models of theory enable you to understand and optimize the system. 

Define a plan and theory. Model and simulate the expected outcomes of behavior based on said theory. The expected outcomes from the modelled representation allows you to understand the system. 

### Improve continously

Flow never arrives at perfection.

Prod the flow. Watch it improve. Prod again. Watch it stumble. Try things, see what happens. Look for waste: pointless checklists, gates that cause problems rather than prevent them. What does not serve the flow is weight the flow carries.

### Dig deeper

Surface-level reasoning rarely captures the truth.

There is an incentive to say "the code is just bad" rather than asking why the code is bad. When analyzing problems in flow, ask why five times. Then solve the smaller, truer problems that emerge.

An agent pushed a change that caused an outage.
1. *Why did the agent push a breaking change?* Because it passed all checks.
2. *Why did the checks pass?* Because there was no integration test covering that path.
3. *Why was there no integration test?* Because the test standard did not require one for that category of change.
4. *Why didn't the standard require it?* Because when the standard was written, that category did not exist yet.
5. *Why wasn't the standard updated?* Because there is no process to review standards when new categories of work are introduced.

The fix is not "make the agent smarter." The fix is a process that revisits standards when the shape of the work changes.

### Fix the system, not the person

Focus on conditions, not fault.

When the AI screws up, there is a pull to blame the AI. Resist it. Analyze the process as a whole. What allowed the failure? Fix the system, not the scapegoat.

---

