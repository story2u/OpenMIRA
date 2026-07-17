import RadarLegacyToken from '../../modules/radar-legacy-token';
import type { LegacyTokenStore } from './tokenMigrationCore';

export const legacyTokenStore: LegacyTokenStore = {
  read: () => RadarLegacyToken.readLegacyTokenAsync(),
  clear: async () => {
    const cleared = await RadarLegacyToken.clearLegacyTokenAsync();
    if (!cleared) throw new Error('Legacy Android token could not be cleared');
  },
};
