import type { ReactNode } from 'react';
import { Button, Group, Text } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../api';
import type { SettingsResponse } from '../types';

// ProxyOff says why something needs the reverse proxy and turns it on in place.
export function ProxyOff({ children }: { children: ReactNode }) {
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const enable = useMutation({
    mutationFn: () => api.put<SettingsResponse>('/settings', { ...settings.data!.settings, proxy_enabled: true }),
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      queryClient.invalidateQueries({ queryKey: ['server'] });
      queryClient.invalidateQueries({ queryKey: ['recording'] });
      if (res.error) notifications.show({ color: 'red', title: "The proxy couldn't start", message: res.error });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Group justify="space-between" wrap="nowrap" gap="sm">
      <Text size="sm" c="dimmed">
        {children}
      </Text>
      <Button size="compact-sm" variant="default" disabled={!settings.data} loading={enable.isPending} onClick={() => enable.mutate()}>
        Turn on proxy
      </Button>
    </Group>
  );
}
