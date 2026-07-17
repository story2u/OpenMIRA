// Re-export the native module. On web, it will be resolved to RadarLegacyTokenModule.web.ts
// and on native platforms to RadarLegacyTokenModule.ts
export { default } from './src/RadarLegacyTokenModule';
export * from './src/RadarLegacyToken.types';
