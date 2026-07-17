import { useFocusEffect } from 'expo-router';
import { useCallback, useState } from 'react';

import { useSession } from '../../auth/SessionProvider';
import { initializeRadarDatabase } from '../../storage/database';
import type { QuietZoneItem } from './quiet-zone-model';
import { readQuietZone } from './quietZoneRepository';

export function useQuietZone() {
  const { state: session } = useSession();
  const ownerId = session.status === 'authenticated' ? session.user.id : null;
  const [items, setItems] = useState<QuietZoneItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(async () => {
    if (!ownerId) return;
    setLoading(true);
    setError(null);
    try {
      setItems(await readQuietZone(await initializeRadarDatabase(), ownerId));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'quiet_zone_failed');
    } finally {
      setLoading(false);
    }
  }, [ownerId]);
  useFocusEffect(useCallback(() => { void load(); }, [load]));
  return { error, items, load, loading };
}

