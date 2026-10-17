import { useState } from 'react';
import { ActionIcon, Badge, Button, Card, Code, Group, NumberInput, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconDownload, IconRestore, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { formatBytes, formatDateTime, summarizeError } from '../../format';
import type { App, Backup } from '../../types';

export function BackupTable({ backups, restorable }: { backups: Backup[]; restorable: boolean }) {
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['backups'] });
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/backups/${id}/restore`),
    onSuccess: () => notifications.show({ color: 'green', message: 'Backup restored' }),
    onError: (err) => notifications.show({ color: 'red', title: 'Restore failed', message: err.message }),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/backups/${id}`),
    onSuccess: refresh,
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  if (backups.length === 0) {
    return (
      <Text c="dimmed" size="sm" px="md" pb="md">
        No backups yet.
      </Text>
    );
  }
  return (
    <Table.ScrollContainer minWidth={520}>
      <Table verticalSpacing="sm">
        <Table.Tbody>
          {backups.map((b) => (
            <Table.Tr key={b.id}>
              <Table.Td>{formatDateTime(b.created_at)}</Table.Td>
              <Table.Td>
                <Badge variant="light" color={b.status === 'success' ? 'green' : b.status === 'failed' ? 'red' : 'blue'}>
                  {b.status}
                </Badge>
                {b.error && (
                  <Text size="xs" c="red" title={b.error}>
                    {summarizeError(b.error)}
                  </Text>
                )}
              </Table.Td>
              <Table.Td>{b.status === 'success' ? formatBytes(b.size) : '—'}</Table.Td>
              <Table.Td>
                <Group gap={4} justify="flex-end" wrap="nowrap">
                  {b.status === 'success' && (
                    <>
                      <Tooltip label="Download">
                        <ActionIcon variant="subtle" color="gray" component="a" href={`/api/backups/${b.id}/download`}>
                          <IconDownload size={16} />
                        </ActionIcon>
                      </Tooltip>
                      {restorable && (
                        <Tooltip label="Restore">
                          <ActionIcon
                            variant="subtle"
                            color="gray"
                            onClick={() =>
                              modals.openConfirmModal({
                                title: 'Restore this backup?',
                                children: (
                                  <Text size="sm">
                                    The app is stopped, its persisted data (shared/) is replaced with this backup, and it's started again.
                                  </Text>
                                ),
                                labels: { confirm: 'Restore', cancel: 'Cancel' },
                                confirmProps: { color: 'red' },
                                onConfirm: () => restore.mutate(b.id),
                              })
                            }
                          >
                            <IconRestore size={16} />
                          </ActionIcon>
                        </Tooltip>
                      )}
                    </>
                  )}
                  <Tooltip label="Delete">
                    <ActionIcon variant="subtle" color="red" onClick={() => remove.mutate(b.id)} disabled={b.status === 'running'}>
                      <IconTrash size={16} />
                    </ActionIcon>
                  </Tooltip>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  );
}

export function Backups({ app }: { app: App }) {
  const backups = useQuery({
    queryKey: ['backups', app.id],
    queryFn: () => api.get<Backup[]>(`/apps/${app.id}/backups`),
    refetchInterval: (q) => (q.state.data?.some((b) => b.status === 'running') ? 2000 : false),
  });
  const create = useMutation({
    mutationFn: () => api.post(`/apps/${app.id}/backups`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups', app.id] }),
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Stack>
      <Card padding={0}>
        <Group justify="space-between" p="md">
          <div>
            <Text fw={600}>Backups</Text>
            <Text size="sm" c="dimmed">
              Backs up the paths listed under <Code>persist</Code> in app.yaml.
            </Text>
          </div>
          <Button onClick={() => create.mutate()} loading={create.isPending}>
            Back up now
          </Button>
        </Group>
        <BackupTable backups={backups.data ?? []} restorable />
      </Card>
      <Schedule app={app} />
    </Stack>
  );
}

function Schedule({ app }: { app: App }) {
  const [cron, setCron] = useState(app.backup_cron);
  const [keep, setKeep] = useState(app.backup_keep);
  const save = useMutation({
    mutationFn: () => api.put<App>(`/apps/${app.id}`, { backup_cron: cron, backup_keep: keep }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['app', app.id] });
      notifications.show({ color: 'green', message: 'Schedule saved' });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Card padding="lg">
      <Text fw={600}>Schedule</Text>
      <Group align="flex-end" mt="md">
        <TextInput
          label="Cron expression"
          description="Server time. Empty = off."
          placeholder="0 3 * * *"
          value={cron}
          onChange={(e) => setCron(e.currentTarget.value)}
          w={260}
        />
        <NumberInput label="Keep" description="Newest backups" min={1} value={keep} onChange={(v) => setKeep(Number(v) || 1)} w={140} />
        <Button onClick={() => save.mutate()} loading={save.isPending}>
          Save
        </Button>
      </Group>
    </Card>
  );
}
