import { useSyncExternalStore } from 'react';

// Whether the panel's API is reachable, so the UI can say "can't reach Moonshed" instead of
// showing whatever error page sits in between (e.g. a dev proxy's "Internal Server Error").
let offline = false;
const listeners = new Set<() => void>();

export function setOffline(value: boolean) {
  if (offline === value) return;
  offline = value;
  listeners.forEach((l) => l());
}

export function useOffline(): boolean {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => offline,
  );
}

export const unreachableMessage = "Can't reach Moonshed. Check that it's running.";
