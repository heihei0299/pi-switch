import { createContext, useContext, type ReactNode } from "react";
import type { ProtocolApiCapability } from "../types";
import { FALLBACK_PROTOCOL_APIS } from "./protocolCapabilities";

// The backend capability set flows from GET /api/state through this context, so
// components and hooks read one value instead of a mutable module global.
const ProtocolContext = createContext<readonly ProtocolApiCapability[]>(FALLBACK_PROTOCOL_APIS);

export function ProtocolProvider({
  apis,
  children,
}: {
  apis?: readonly ProtocolApiCapability[];
  children: ReactNode;
}) {
  const value = apis && apis.length > 0 ? apis : FALLBACK_PROTOCOL_APIS;
  return <ProtocolContext.Provider value={value}>{children}</ProtocolContext.Provider>;
}

export function useProtocolCapabilities(): readonly ProtocolApiCapability[] {
  return useContext(ProtocolContext);
}
