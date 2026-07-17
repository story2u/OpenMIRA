import { Redirect, type Href } from 'expo-router';

export default function LegacyHomeRedirect() {
  return <Redirect href={'/(tabs)/home' as Href} />;
}
