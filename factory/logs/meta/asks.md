> canonical customer-ask surface: edit and prioritize this backlog only in
> `factory/logs/meta/asks.md`. If another path mentions customer asks, treat
> this file as the source of truth.

# customer asks (look to prioritize as necessary, do one thing at a time)

we look generally to make the system amenable to consumption via other customers beyond us. 

## Import/Export problems (P0); 

### Functionality 
we are testing out the import and export functionality, and it works generally but there are issues. 

problems: 
- the get factory API, is has body vs template, which is strange, we should only have one and remove the other for the workstation. 
Please remove the promptTemplate. 

- when importing a factory, all the fields are flattened but also expanded at the same time. 
-- this is confusing, it should be expanded out, and the top level fields at the factory.json should be thinner: 
--- (Workers) we should shove out all the body into the AGENTS.md file, and the AGENTS.md file should not have anything else besides the body
--- (workstations) we should shove out all the body into the AGENTS.md file, and the AGENTS.md file should not have anything else besides the body

Please add tests to confirm this general behavior. 

#### Bundled files
The bundled files currently require in the factory.json to declare the inline
          "content": {
            "encoding": "utf-8"          }

This is very strange: 
- we should not require the content to be declared, as it should be resolved to real file that is on disk. 
- we should make the file be expanded out, and not be persisted on the factory.json when it gets imported. 

During the query for the factory.json we should include the factory scripts/docs, by default, and we should add tests to confirm that this works. Similarly, we should add tests to confirm that import flattens out the supported bundledFiles to be broken out, and have the content not be inlined. 

### Dialogues

The current import/export dialogues that appear when importing a factory is strangely put inside the workflow-activity, it should be exposed as a separate dialogue directly. 
The buttons should be converged onto the default shadcn buttons styling. 

## quality (P0)
- we need to improve our overall system quality, to reduce future rework rates and what not
please look towards implementing our systems and moving towards the standards outlined for both the website and the backend. Generally, keep a checklist as part of the progress towards migration our systems towards alignment with the standards denoted at docs/standards/code/general-website-standards.md and  docs/standards/code/general-backend-standards.md 

### website quality (more details - p0/p1 - follow on from quality)
#### website testing
we need more concretely higher tests, we have some tests, but we should cover more, target 90% overall and declare that as the new bare minimum

#### website button confusion
the buttons are all different colors, we should make all the buttons the same color, to reduce the noise. we should not make them the primary color for now, we should make them the color of white that we use for the drag handles color. 

#### work current selection confusion

when making a selection for a workstaiton work, there is far too much noise: 
the request prompt and information is duplicated like 3 different times. 

- the workstation request projection section
- the show inferene attempts section
- frankly its all insane

please just purge it down into a very basic thing: 

this is the name/id of the work, along with the work type
- here is the relationship graph of the work
- here is a list of dispatches that are associated with the work
-- for each dispatch here is a list of inference request/responses, script request/response
--- for each inference request/response, denote the provider session id, the request/response body/response, worktree, working directory, the provider/model, elapsed time. 

#### Submit work
The submit work form is too versbose: 

"- Send a new request to the current factory from the dashboard."
should be removed


### Website bugs/issues (P1)
#### work totals
the work total outcomes has the chart which is okay, but the labels are separate from the chart? it should be merged in as child of the axes, we should add tests for the rendered chart to validate it has the legend and axes labels

https://stackoverflow.com/questions/55292211/how-to-show-label-name-of-a-data-in-y-axis-in-recharts

similarly, the chart is as wide as its containing bento box, which is bad, we should have some spacing to the left/right axes. 

#### Icon

The current Icon is a triangle on a black box, which was fine at the beginning but now we've renamed to "infinite-you", so please make the Icon a glowing "infinity". The "Agent Factory" header should be renamed to an inifinity symbol + a U. 

#### Verbosity
Factory state -> Running, Stream (Factory event stream connected), export PNG, Timeline Tick

That's too many words: 

##### state/stream state

-> drop the factory state from the list

 and stream just to just pulsating small circles
it should be a small green pulsating icon when its on, and should be red pulsating icon when its not connected 
https://magicui.design/docs/components/pulsating-button, please look at the discord icon on this page for reference.

##### export PNG

-> This should just be a share ICON, no text

##### Timeline Tick
The text should be jremoved, the current on the text button should just be a Play icon




### docs audit (p2)
we should audit our docs, some of it is stale and what not, especially the ones we embed as part of our systems cli, we should try to keep those up to date for now

### manual qa (p2)
we should try to run a few (20) manual qa runs with mock runners with a variety of possible schemas of factories and feedback into our plans on how things should be fixed. 

### systems quality (p2)

we are lacking some copmponents in our docs for systems quality

namely

1. we need a doc that explains our layers of data models
- low level baseline (petri nets/guards/colors/tokens/transitions), the higher level abstractions (workers/work/workstatinos), what is the conflation between the two systems, and why we separate (mostly because the low level is too verbose/complex to expose directly, but that the formalisms provided is nice and clean)

2. we need a doc that defines code quality standards,
- quality is about conformance to simplicity, and we prefer simplicity of the system over making a small seam change. 
- we prefer having systems be easily reasoned about, and look to always have as minimal state as possible. We look to make as much of our systems referentially transparent
- we look to automate quality, because that makes it transparent

