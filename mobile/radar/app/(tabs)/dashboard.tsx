import DashboardScreen from '../../src/features/dashboard/DashboardScreen';
import SmartMessagesScreen from '../../src/features/messages/SmartMessagesScreen';
import { isMiraConciergeUiEnabled } from '../../src/config/miraConciergeFlag';
import { createDefaultDashboardFilters } from '../../src/features/dashboard/filters';
import { useLocalSearchParams } from 'expo-router';

function filtersForCategory(category: string | string[] | undefined) {
  const filters = createDefaultDashboardFilters();
  const value = Array.isArray(category) ? category[0] : category;
  if (value === 'pending') filters.status = 'pending';
  if (value === 'business' || value === 'jobs') filters.sort = 'confidence';
  return filters;
}

export default function DashboardRoute() {
  const params = useLocalSearchParams<{ category?: string; view?: string }>();
  if (!isMiraConciergeUiEnabled() || params.view === 'list') {
    return <DashboardScreen initialFilters={filtersForCategory(params.category)} />;
  }
  return <SmartMessagesScreen />;
}
