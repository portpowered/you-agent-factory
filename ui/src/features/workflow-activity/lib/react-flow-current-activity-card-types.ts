export type CurrentActivitySelection =
  | { kind: "node"; nodeId: string }
  | { kind: "state-node"; placeId: string }
  | { kind: "worker"; workerName: string }
  | { kind: "resource"; resourceName: string }
  | { kind: "work-type"; workTypeName: string }
  | { kind: "work-item"; dispatchId: string; nodeId: string; workID: string };
