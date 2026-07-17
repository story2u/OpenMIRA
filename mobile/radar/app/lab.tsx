import { Redirect } from 'expo-router';

import CompatibilityLab from '../App';

export default function LabScreen() {
  if (!__DEV__) return <Redirect href="/" />;
  return <CompatibilityLab />;
}
