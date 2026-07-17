import { Redirect } from 'expo-router';

export default function LegacyHomeRedirect() {
  return <Redirect href="/(tabs)/dashboard" />;
}
