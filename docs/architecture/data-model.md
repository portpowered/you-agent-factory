# Data model

This document captures the Agent Factory data model and separates entities into two layers:
- Customer-facing surface
- Factory internals

The data model has a customer facing data model and an internal data model. 

The internal data model is what is defined in business processes a "petri net with colored tokens and guarded transitions" 
The factory customer data model is an abstraction over that petri net that makes customers lives easier to deal with. The raw tokens/transitions ends up far too verbose to express reasonably. 

## Internal system data model

```mermaid
---
title: Agent Factory Data Model
config:
  layout: elk
---
erDiagram
    direction TB

    classDef factory fill:#ECFCCB,stroke:#4D7C0F,stroke-width:2px,color:#365314
    
    tok["Token"] {
        string id
        map colors
    }
    trans["Transition"] {
        string id
    }
    p["Place"] {
        string id
    }
    e["Edge"] {
        string id
    }
    g["Guard"] {
        string id
    }

    
    g }o--o| e: "prevents the transition of tokens in"
    e }o--o| p: "moves tokens in and out from transitions"
    e }o--o| trans: "is connected to places via"
    tok }o--o| p: "is located at"

    class tok,trans,g factory
```

## Customer data model 

This denotes the data model that we express to customers. Customers deal with work that goes into a factory. The factory has workstations that take in work. The factory/workstations have guards that protect the individual work. 

```mermaid
---
title: Agent Factory Data Model
config:
  layout: elk
---
erDiagram
    direction TB

    classDef customer fill:#E0F2FE,stroke:#0369A1,stroke-width:2px,color:#0C4A6E
    classDef factory fill:#ECFCCB,stroke:#4D7C0F,stroke-width:2px,color:#365314
    classDef workload fill:#FFF7ED,stroke:#C2410C,stroke-width:2px,color:#7C2D12
    classDef history fill:#EDE9FE,stroke:#6D28D9,stroke-width:2px,color:#432875

    %% Customer-facing surface
    cust["Customer"] {
        string customerId
        string name
    }
    w["Work"] {
        string workId
        string status
        datetime createdAt
    }
    rel["Relationship"] {
        string type
    }

    g["guard"] {
        string type
    }
    w }|--o{ rel: "has"
    %% Factory internals
    f["Factory"] {
        string factoryId
    }
    wt["WorkType"] {
        string name
    }
    ws["Workstation"] {
        string name
    }
    wr["Worker"] {
        string name
    }
    r["Resource"] {
        string name
    }
    f ||--o{ wt : "runs"
    f ||--o{ ws : "contains"
    f ||--o{ wr : "operates"
    f ||--o{ r : "owns"

    wt }o--|| w: "defines"
    w }o--o{ ws : "scheduled on"
    wr }o--o| ws : "assigned to"
    wr }o--o{ r: "uses"
    ws }o--o{ r: "provisions"

    ws }o--o{ g : "has"
    f }o--o{ g : "has"

    %% Work request/response lane
    req["Workstation Request"] {
        string requestId
        string payload
    }
    resp["Workstation Response"] {
        string responseId
        string status
    }

    chg["Work Change"] {
    }

    chg }o--o{ w: "modifies"
    cust }o--o{ chg: "submits"
    chg }o--o{ f: "submit"

    ws ||--o{ req: "submits"
    w ||--o{ req: "produced by"
    r ||--o{ req: "enables"
    wr ||--o{ req: "executes"
    req ||--|| resp: "returns"
    resp }o--o{ w: "updates"
    

    %% Work history/audit layer
    wh["Work History"] {
        datetime startedAt
        datetime finishedAt
        string result
    }
    req }o--|| wh: "recorded in"
    resp }o--|| wh: "recorded in"
    chg }o--|| wh: "recorded in"
    class cust customer
    class f,wt,ws,wr,r,g factory
    class w,rel,chg,req,resp workload
    class wh history
```

