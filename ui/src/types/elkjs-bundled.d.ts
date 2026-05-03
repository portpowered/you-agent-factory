declare module "elkjs/lib/elk.bundled.js" {
  export interface LayoutOptions {
    [key: string]: string;
  }

  export interface ElkPoint {
    x: number;
    y: number;
  }

  export interface ElkShape {
    height?: number;
    id?: string;
    layoutOptions?: LayoutOptions;
    width?: number;
    x?: number;
    y?: number;
  }

  export interface ElkNode extends ElkShape {
    children?: ElkNode[];
    edges?: ElkExtendedEdge[];
    id: string;
  }

  export interface ElkExtendedEdge {
    id: string;
    sections?: ElkEdgeSection[];
    sources: string[];
    targets: string[];
  }

  export interface ElkEdgeSection {
    bendPoints?: ElkPoint[];
    endPoint: ElkPoint;
    id: string;
    startPoint: ElkPoint;
  }

  export interface ELK {
    layout<T extends ElkNode>(
      graph: T,
    ): Promise<Omit<T, "children"> & { children?: Array<T["children"] extends Array<infer U> ? U & ElkNode : ElkNode> }>;
  }

  const ElkConstructor: {
    new(): ELK;
  };

  export default ElkConstructor;
}

