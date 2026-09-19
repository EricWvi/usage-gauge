export type Tier = {
  name: string;
  label?: string;
  utilization: number;
  used?: number;
  limit?: number;
  unit?: string;
  resetsAt?: string;
};
export type Sample = {
  name: string;
  updatedAt: number;
  queriedAt: number;
  status: "ok" | "expired" | "error";
  message?: string;
  error?: string;
  tiers: Tier[];
};
export type Endpoint = {
  name: string;
  provider: string;
  latest: Sample | null;
  history: Sample[];
};
export type Dashboard = {
  serverTime: number;
  lastUpdatedAt: number;
  sampleIntervalMs: number;
  retentionHours: number;
  endpoints: Endpoint[];
};
