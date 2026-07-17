export interface NetworkReachability {
  isConnected?: boolean;
  isInternetReachable?: boolean;
}

type Reachability = 'offline' | 'online' | 'unknown';

function classify(state: NetworkReachability): Reachability {
  if (state.isConnected === false || state.isInternetReachable === false) return 'offline';
  if (state.isConnected === true) return 'online';
  return 'unknown';
}

export interface NetworkRecoveryDetector {
  observe(state: NetworkReachability): boolean;
  seed(state: NetworkReachability): void;
}

export function createNetworkRecoveryDetector(): NetworkRecoveryDetector {
  let previous: Reachability = 'unknown';
  return {
    seed(state) {
      const next = classify(state);
      if (next !== 'unknown') previous = next;
    },
    observe(state) {
      const next = classify(state);
      if (next === 'unknown') return false;
      const recovered = previous === 'offline' && next === 'online';
      previous = next;
      return recovered;
    },
  };
}
