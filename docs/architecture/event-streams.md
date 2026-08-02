# Event Streams

Systems correspond roughly to the construction of event streams.

there are two general event streams that are high event procedures.

1. factory events
2. worker events

## factory events

Factory events denote the system state of the factory, what is next, what is going on.
At any given time, you can largely replay the world state of the factory to the current tick of the event stream it is operating on.

- Factory Created (Definition)
- Factory Start
    - ===============================
    - Workstation Start
        - Worker Start
            - Model Request Start
            - Model Request Stream
            - Model Request End  (Complete/Fail)
            - Script Request Start
            - Script Request Stream
            - Script Request End  (Complete/Fail)
            - Agent Request Start
            - Agent Request Stream
            - Agent Request End  (Complete/Fail)
            - Logic Request Start
            - Logic Request Stream
            - Logic Request End  (Complete/Fail)
        - Worker End (Complete/Fail)
    - Workstation End (Complete/Fail)
    - ================================
    - Work Submission Request Start
    - Work Submission request End (Complete/Fail)
    - Work Change Request Start
    - Work Change Request End (Complete/Fail)
    - ================================
    - Factory Definition Update Start (Definition change)
    - Factory Definition Update End (Complete/Fail)
    - Factory State Update Request Start (Paused/Running/Created/Started/Failed)
    - Factory State Update Request End (Complete/Failed)
- Factory End
- Error
## worker session events

Worker events denote the changes that occur within the context of a given worker request

- Session Start
- User Stream Start
    - User Stream Update
        - User Item added
            - User content start
            - User content stream
                - thinking content
                - text content
                - audio/image/file content
                - tool call content
            - User Content end
        - User item added
- User Stream End
- Agent stream Start
    - Agent Stream Update
        - Agent Item added
            - Agent content start
            - Agent content stream
                - thinking content
                - text content
                - audio/image/file content
                - tool call content
            - Agent content end
        - Agent Item end
- Agent Stream End
- Tool stream Start
    - Tool Stream Update
        - Tool Item added
            - Tool content start
            - Tool content stream
                - thinking content
                - text content
                - audio/image/file content
                - tool call content
            - Tool content end
        - Tool Item end
- Tool Stream End
- Session In pRogress
- Session Failed
- Session End
- Error