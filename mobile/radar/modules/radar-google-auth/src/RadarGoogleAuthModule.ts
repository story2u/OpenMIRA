import { NativeModule, requireOptionalNativeModule } from 'expo';

import type { RadarGoogleAuthResult } from './RadarGoogleAuth.types';

declare class RadarGoogleAuthModule extends NativeModule<{}> {
  signInAsync(serverClientId: string, iosClientId: string | null): Promise<RadarGoogleAuthResult>;
}

export default requireOptionalNativeModule<RadarGoogleAuthModule>('RadarGoogleAuth');
