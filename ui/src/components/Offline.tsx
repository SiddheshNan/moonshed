import { useEffect } from 'react';
import { Center, Loader, Paper, Stack, Text } from '@mantine/core';

import { queryClient } from '../api';
import { setOffline, unreachableMessage, useOffline } from '../connection';

// useReconnect polls the API while it's unreachable and refreshes everything once it's back.
function useReconnect(active: boolean) {
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(async () => {
      try {
        const res = await fetch('/api/auth/status');
        if (res.headers.get('Content-Type')?.includes('application/json')) {
          setOffline(false);
          queryClient.invalidateQueries();
        }
      } catch {
        // still down
      }
    }, 3000);
    return () => clearInterval(timer);
  }, [active]);
}

// OfflineBanner sits above the app while the API is unreachable.
export function OfflineBanner() {
  const offline = useOffline();
  useReconnect(offline);
  if (!offline) return null;
  return (
    <div className="offline-banner">
      <Loader size={12} color="orange" />
      <Text size="sm">{unreachableMessage} Retrying…</Text>
    </div>
  );
}

// Unreachable replaces the whole page when the API can't be reached before anything loaded.
export function Unreachable() {
  useReconnect(true);
  return (
    <Center mih="100vh" p="md" className="auth-backdrop">
      <Paper withBorder p="xl" maw={420} bg="var(--app-surface)">
        <Stack gap="xs" align="center">
          <Loader size="sm" />
          <Text fw={600}>Can't reach Moonshed</Text>
          <Text size="sm" c="dimmed" ta="center">
            Check that it's running. This page reconnects on its own.
          </Text>
        </Stack>
      </Paper>
    </Center>
  );
}
