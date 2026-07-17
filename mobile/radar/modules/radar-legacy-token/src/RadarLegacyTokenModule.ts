import { NativeModule, requireNativeModule } from 'expo';

declare class RadarLegacyTokenModule extends NativeModule<{}> {
  readLegacyTokenAsync(): Promise<string | null>;
  clearLegacyTokenAsync(): Promise<boolean>;
}

export default requireNativeModule<RadarLegacyTokenModule>('RadarLegacyToken');
