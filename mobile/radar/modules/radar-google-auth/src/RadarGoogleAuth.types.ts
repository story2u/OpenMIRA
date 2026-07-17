export type RadarGoogleAuthResult =
  | { type: 'cancelled' }
  | { type: 'success'; idToken: string };
