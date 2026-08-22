export interface GraphNode {
  id: number;
  x: number;
  y: number;
  z: number;
  label: string;
  name: string;
  qualified_name: string;
  file_path?: string;
  start_line?: number;
  end_line?: number;
  degree: number;
  size: number;
  color: string;
  community?: string;
}

export interface GraphEdge {
  source: number;
  target: number;
  type: string;
}

export interface GraphData {
  nodes: GraphNode[];
  edges: GraphEdge[];
  total_nodes: number;
  total_edges: number;
  project?: string;
}

export interface GraphDisplaySettings {
  edgeBrightness: number;
  nodeGlow: number;
  bloom: number;
}

export const DEFAULT_GRAPH_DISPLAY: GraphDisplaySettings = {
  edgeBrightness: 1,
  nodeGlow: 1,
  bloom: 0,
};
