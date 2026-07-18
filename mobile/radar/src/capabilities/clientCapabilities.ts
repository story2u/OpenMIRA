import type { ClientCapabilities } from '@story2u/radar-contracts/devices';

export type { ClientCapabilities } from '@story2u/radar-contracts/devices';

export const disabledCapabilities: Readonly<ClientCapabilities> = Object.freeze({
  agentToolsAvailable: false,
  deviceAgentAvailable: false,
  e2eeAvailable: false,
  hostedFallbackAvailable: false,
  pushAvailable: false,
  rnClientSupported: false,
  syncAvailable: false,
  signalAppetiteSyncAvailable: false,
});

export function parseCapabilities(value: unknown): ClientCapabilities {
  if (!value || typeof value !== 'object') return { ...disabledCapabilities };
  const source = value as Record<string, unknown>;
  return Object.fromEntries(
    Object.keys(disabledCapabilities).map((key) => [key, source[key] === true]),
  ) as unknown as ClientCapabilities;
}
