/** Node-kind colors shared by the scene tooltip, labels UI, and filter chips. */
export const LABEL_COLORS: Record<string, string> = {
  Project: "#e11d48",
  Package: "#f97316",
  Module: "#f97316",
  Folder: "#22c55e",
  File: "#3b82f6",
  Class: "#a855f7",
  Interface: "#a855f7",
  Function: "#06b6d4",
  Method: "#06b6d4",
  Route: "#eab308",
  Variable: "#64748b",
  Prompt: "#06b6d4",
  Tool: "#3b82f6",
  Session: "#eab308",
  Pin: "#ec4899",
  Teaching: "#22c55e",
  Working: "#f97316",
  Memory: "#a855f7",
};

export const DEFAULT_LABEL_COLOR = "#94a3b8";

export function labelColor(label: string): string {
  return LABEL_COLORS[label] ?? DEFAULT_LABEL_COLOR;
}

/** Relationship colors for the additive edge field. */
export const EDGE_COLORS: Record<string, string> = {
  CALLS: "#1DA27E",
  CALL_REFERENCE: "#14b8a6",
  IMPORTS: "#3b82f6",
  DEFINES: "#a855f7",
  DEFINES_METHOD: "#a855f7",
  CONTAINS_FILE: "#22c55e",
  CONTAINS_FOLDER: "#22c55e",
  CONTAINS_PACKAGE: "#22c55e",
  HANDLES: "#eab308",
  IMPLEMENTS: "#f97316",
  HTTP_CALLS: "#e11d48",
  ASYNC_CALLS: "#ec4899",
  GRPC_CALLS: "#f59e0b",
  GRAPHQL_CALLS: "#e879f9",
  TRPC_CALLS: "#a78bfa",
  CROSS_HTTP_CALLS: "#fb923c",
  CROSS_ASYNC_CALLS: "#fb7185",
  CROSS_GRPC_CALLS: "#fbbf24",
  CROSS_GRAPHQL_CALLS: "#f0abfc",
  CROSS_TRPC_CALLS: "#c4b5fd",
  CROSS_CHANNEL: "#fdba74",
  MEMBER_OF: "#64748b",
  TESTS: "#06b6d4",
  TESTS_FILE: "#06b6d4",
  USAGE: "#64748b",
  CONFIGURES: "#f59e0b",
  contradicts: "#ef4444",
  same_session: "#737373",
  rolled_up_from: "#d6d3d1",
  taught_from: "#a8a29e",
  next: "#94a3b8",
};

export const DEFAULT_EDGE_COLOR = "#1C8585";
