/** Shared application and runtime contracts for the NAP-STEAMDUCK v1 draft. */
export interface BridgeInfo {
  version: number;
  methods: string[];
  kwakore_protocol_version: number | null;
}

export interface SteamduckAPI {
  info(): Promise<BridgeInfo>;
  call<T = unknown>(method: string, params?: Record<string, unknown> | null): Promise<T>;
  kwakore: { call<T = unknown>(method: string, params?: Record<string, unknown> | null): Promise<T> };
}

export interface PublicError {
  code: number;
  message: string;
  reason?: string;
  data?: Record<string, unknown>;
}

export type RequestType = "steamduck.info" | "steamduck.request" | "steamduck.kwakore.request";

export interface RequestEnvelope {
  type: RequestType;
  id: string;
  method?: string;
  params?: Record<string, unknown> | null;
}
