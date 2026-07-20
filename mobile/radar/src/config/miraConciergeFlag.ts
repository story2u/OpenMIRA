export function isMiraConciergeUiEnabled() {
  const raw = process.env.EXPO_PUBLIC_MIRA_CONCIERGE_UI_ENABLED?.trim().toLowerCase();
  if (raw === '0' || raw === 'false' || raw === 'off') return false;
  if (raw === '1' || raw === 'true' || raw === 'on') return true;
  return __DEV__;
}
