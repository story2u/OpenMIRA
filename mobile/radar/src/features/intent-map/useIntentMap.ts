import { useFocusEffect } from 'expo-router';
import { useCallback, useState } from 'react';

import { useSession } from '../../auth/SessionProvider';
import { initializeRadarDatabase } from '../../storage/database';
import type { IntentMapModel } from './intent-map-model';
import { readIntentMapModel } from './intentMapRepository';

export function useIntentMap() {
  const { state: session } = useSession();
  const ownerId = session.status === 'authenticated' ? session.user.id : null;
  const [model, setModel] = useState<IntentMapModel | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(async () => {
    if (!ownerId) return;
    setLoading(true);
    setError(null);
    try {
      const database = await initializeRadarDatabase();
      setModel(await readIntentMapModel(database, ownerId));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'intent_map_failed');
    } finally {
      setLoading(false);
    }
  }, [ownerId]);
  useFocusEffect(useCallback(() => {
    void load();
  }, [load]));
  return { error, load, loading, model };
}
