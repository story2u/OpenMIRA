import { registerWebModule, NativeModule } from 'expo';

// RadarLegacyTokenModule is not available on the web platform.
class RadarLegacyTokenModule extends NativeModule<{}> {
  async readLegacyTokenAsync(): Promise<null> {
    return null;
  }

  async clearLegacyTokenAsync(): Promise<boolean> {
    return false;
  }
}

export default registerWebModule(RadarLegacyTokenModule, 'RadarLegacyTokenModule');
