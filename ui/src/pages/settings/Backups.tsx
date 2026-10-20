import { useState } from 'react';
import { Button, Card, Code, Group, NumberInput, Stack, Switch, Text, TextInput } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import type { Backup, Settings, SettingsResponse } from '../../types';
import { BackupTable } from '../app/Backups';

export function BackupsSettingsPage() {
  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Backups" />
      <PlatformBackups />
    </Stack>
  );
}

function PlatformBackups() {
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const backups = useQuery({ queryKey: ['backups', 'platform'], queryFn: () => api.get<Backup[]>('/platform/backups') });
  const create = useMutation({
    mutationFn: () => api.post('/platform/backups'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups', 'platform'] }),
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Stack gap="lg">
      <Card padding={0}>
        <Group justify="space-between" className="panel-header" wrap="nowrap">
          <div>
            <Text fw={600}>Platform backups</Text>
            <Text size="sm" c="dimmed">
              Database and encryption key. To restore, stop Moonshed and extract the archive into <Code>runtime/data/</Code>.
            </Text>
          </div>
          <Button variant="default" onClick={() => create.mutate()} loading={create.isPending}>
            Back up now
          </Button>
        </Group>
        <BackupTable backups={backups.data ?? []} />
      </Card>
      {settings.data && <PlatformSchedule key={JSON.stringify(settings.data.settings)} stored={settings.data.settings} />}
    </Stack>
  );
}

function PlatformSchedule({ stored }: { stored: Settings }) {
  const [enabled, setEnabled] = useState(stored.backup_enabled);
  const [cron, setCron] = useState(stored.backup_cron);
  const [keep, setKeep] = useState(stored.backup_keep);
  const save = useMutation({
    mutationFn: () => api.put<SettingsResponse>('/settings', { ...stored, backup_enabled: enabled, backup_cron: cron, backup_keep: keep }),
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      notifications.show({ color: 'green', message: 'Schedule saved' });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const changed = enabled !== stored.backup_enabled || cron !== stored.backup_cron || keep !== stored.backup_keep;

  return (
    <Card padding={0}>
      <Group justify="space-between" className="panel-header">
        <Text fw={600}>Platform backup schedule</Text>
        {changed && (
          <Button size="sm" onClick={() => save.mutate()} loading={save.isPending} disabled={enabled && !cron.trim()}>
            Save
          </Button>
        )}
      </Group>
      <Stack p="lg">
        <Switch label="Back up on a schedule" checked={enabled} onChange={(e) => setEnabled(e.currentTarget.checked)} />
        {enabled && (
          <Group align="flex-start">
            <TextInput
              label="Cron expression"
              description="Server time, e.g. 0 2 * * * for 2:00 every day"
              placeholder="0 2 * * *"
              required
              value={cron}
              onChange={(e) => setCron(e.currentTarget.value)}
              error={!cron.trim() ? 'Required' : undefined}
              w={300}
            />
            <NumberInput label="Keep" description="Newest backups" min={1} value={keep} onChange={(v) => setKeep(Number(v) || 1)} w={140} />
          </Group>
        )}
      </Stack>
    </Card>
  );
}
