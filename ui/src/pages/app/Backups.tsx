import { useState } from 'react';
import { ActionIcon, Badge, Button, Card, Group, Modal, NumberInput, SimpleGrid, Stack, Switch, Table, Text, TextInput, Tooltip } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconDownload, IconRestore, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { TimeAgo } from '../../components/TimeAgo';
import { formatBytes, summarizeError } from '../../format';
import type { App, Backup, BackupParts } from '../../types';
import { Section, useSave } from './Settings';

type Part = keyof BackupParts;

const parts: { key: Part; label: string; hint: string }[] = [
  { key: 'code', label: 'Code', hint: 'The deployed version' },
  { key: 'data', label: 'Data', hint: 'Persisted paths' },
  { key: 'env', label: 'Env vars', hint: 'From the Environment tab' },
  { key: 'logs', label: 'Logs', hint: 'App logs and the deploy log' },
];

const partNames = (p: BackupParts) => parts.filter((x) => p[x.key]).map((x) => x.label);

function PartSwitches({
  value,
  onChange,
  only,
  disabled = {},
}: {
  value: BackupParts;
  onChange: (v: BackupParts) => void;
  only?: Part[];
  disabled?: Partial<Record<Part, string>>;
}) {
  return (
    <SimpleGrid cols={{ base: 1, xs: 2 }} spacing="sm">
      {parts
        .filter((p) => !only || only.includes(p.key))
        .map((p) => (
          <Switch
            key={p.key}
            label={p.label}
            description={disabled[p.key] ?? p.hint}
            checked={value[p.key] && !disabled[p.key]}
            disabled={!!disabled[p.key]}
            onChange={(e) => onChange({ ...value, [p.key]: e.currentTarget.checked })}
          />
        ))}
    </SimpleGrid>
  );
}

const anyPart = (p: BackupParts) => p.code || p.data || p.env || p.logs;

export function BackupTable({ backups, app }: { backups: Backup[]; app?: App }) {
  const [restoring, setRestoring] = useState<Backup | null>(null);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['backups'] });
  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/backups/${id}`),
    onSuccess: refresh,
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  const download = (b: Backup) => {
    const go = () => {
      location.href = `/api/backups/${b.id}/download`;
    };
    if (!b.parts?.env) return go();
    modals.openConfirmModal({
      title: 'Download this backup?',
      children: <Text size="sm">It contains the app's env vars in plain text, in its .env file. Keep the download somewhere safe.</Text>,
      labels: { confirm: 'Download', cancel: 'Cancel' },
      onConfirm: go,
    });
  };

  if (backups.length === 0) {
    return (
      <Text c="dimmed" size="sm" className="empty-state">
        No backups yet.
      </Text>
    );
  }
  return (
    <>
      <Table.ScrollContainer minWidth={560}>
        <Table verticalSpacing="sm">
          <Table.Tbody>
            {backups.map((b) => (
              <Table.Tr key={b.id}>
                <Table.Td className="nowrap">
                  <TimeAgo iso={b.created_at} />
                </Table.Td>
                <Table.Td>
                  {b.status === 'success' ? (
                    app && (
                      <Group gap={4}>
                        {partNames(b.parts).map((n) => (
                          <Badge key={n} variant="default" fw={500}>
                            {n}
                          </Badge>
                        ))}
                      </Group>
                    )
                  ) : (
                    <>
                      <Badge color={b.status === 'failed' ? 'red' : 'blue'}>{b.status === 'failed' ? 'Failed' : 'Running'}</Badge>
                      {b.error && (
                        <Text size="xs" c="red" title={b.error}>
                          {summarizeError(b.error)}
                        </Text>
                      )}
                    </>
                  )}
                </Table.Td>
                <Table.Td c="dimmed" className="nowrap">
                  {b.status === 'success' ? formatBytes(b.size) : '—'}
                </Table.Td>
                <Table.Td>
                  <Group gap="xs" justify="flex-end" wrap="nowrap">
                    {b.status === 'success' && app && (
                      <Button size="compact-sm" variant="default" leftSection={<IconRestore size={14} />} onClick={() => setRestoring(b)}>
                        Restore
                      </Button>
                    )}
                    {b.status === 'success' && (
                      <Button size="compact-sm" variant="default" leftSection={<IconDownload size={14} />} onClick={() => download(b)}>
                        Download
                      </Button>
                    )}
                    <Tooltip label="Delete">
                      <ActionIcon color="red" onClick={() => remove.mutate(b.id)} disabled={b.status === 'running'}>
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
      {app && <RestoreModal app={app} backup={restoring} onClose={() => setRestoring(null)} />}
    </>
  );
}

function RestoreModal({ app, backup, onClose }: { app: App; backup: Backup | null; onClose: () => void }) {
  const has = backup?.parts ?? { code: false, data: false, env: false, logs: false };
  const [choice, setChoice] = useState<BackupParts>({ code: false, data: has.data, env: false, logs: false });
  const [loaded, setLoaded] = useState(backup);
  if (backup !== loaded) {
    setLoaded(backup);
    setChoice({ code: false, data: !!backup?.parts.data, env: false, logs: false });
  }
  const restore = useMutation({
    mutationFn: () => api.post(`/backups/${backup!.id}/restore`, choice),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['app', app.id] });
      queryClient.invalidateQueries({ queryKey: ['deployments', app.id] });
      notifications.show({ color: 'green', message: choice.code ? 'Restored. The code is being deployed.' : 'Backup restored' });
      onClose();
    },
    onError: (err) => notifications.show({ color: 'red', title: 'Restore failed', message: err.message }),
  });
  const available = (['code', 'data', 'env'] as Part[]).filter((p) => has[p]);

  return (
    <Modal opened={backup !== null} onClose={onClose} title="Restore backup">
      <Stack>
        <PartSwitches value={choice} onChange={setChoice} only={available} />
        <Text size="xs" c="dimmed">
          {choice.data && 'Data replaces the persisted paths; the app is stopped meanwhile. '}
          {choice.env && 'Env vars replace the Environment tab. '}
          {choice.code && 'Code is deployed as a new deployment.'}
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button color="red" disabled={!choice.code && !choice.data && !choice.env} loading={restore.isPending} onClick={() => restore.mutate()}>
            Restore
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

function BackupNowModal({ app, opened, onClose }: { app: App; opened: boolean; onClose: () => void }) {
  const [choice, setChoice] = useState<BackupParts>(app.backup_parts);
  const noCode = app.current_release_id ? undefined : 'Not deployed yet';
  const create = useMutation({
    mutationFn: () => api.post(`/apps/${app.id}/backups`, { ...choice, code: choice.code && !noCode }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['backups', app.id] });
      onClose();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const valid = anyPart({ ...choice, code: choice.code && !noCode });

  return (
    <Modal opened={opened} onClose={onClose} title={`Back up ${app.name}`}>
      <Stack>
        <PartSwitches value={choice} onChange={setChoice} disabled={{ code: noCode }} />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!valid} loading={create.isPending} onClick={() => create.mutate()}>
            Back up
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

export function Backups({ app }: { app: App }) {
  const [creating, setCreating] = useState(false);
  const backups = useQuery({
    queryKey: ['backups', app.id],
    queryFn: () => api.get<Backup[]>(`/apps/${app.id}/backups`),
    refetchInterval: (q) => (q.state.data?.some((b) => b.status === 'running') ? 2000 : false),
  });

  return (
    <Stack gap="lg">
      <Card padding={0}>
        <Group justify="space-between" className="panel-header">
          <Text fw={600}>Backups</Text>
          <Button onClick={() => setCreating(true)}>Back up now</Button>
        </Group>
        <BackupTable backups={backups.data ?? []} app={app} />
      </Card>
      <Schedule key={`${app.backup_enabled}-${app.backup_cron}-${app.backup_keep}-${JSON.stringify(app.backup_parts)}`} app={app} />
      <BackupNowModal key={String(creating)} app={app} opened={creating} onClose={() => setCreating(false)} />
    </Stack>
  );
}

function Schedule({ app }: { app: App }) {
  const save = useSave(app);
  const [enabled, setEnabled] = useState(app.backup_enabled);
  const [cron, setCron] = useState(app.backup_cron);
  const [keep, setKeep] = useState(app.backup_keep);
  const [choice, setChoice] = useState<BackupParts>(app.backup_parts);
  const changed =
    enabled !== app.backup_enabled ||
    cron !== app.backup_cron ||
    keep !== app.backup_keep ||
    JSON.stringify(choice) !== JSON.stringify(app.backup_parts);
  const invalid = enabled && (!cron.trim() || !anyPart(choice));

  return (
    <Section
      title="Schedule"
      onSave={changed ? () => save.mutate({ backup_enabled: enabled, backup_cron: cron, backup_keep: keep, backup_parts: choice }) : undefined}
      saveDisabled={invalid}
      saving={save.isPending}
    >
      <Switch label="Back up on a schedule" checked={enabled} onChange={(e) => setEnabled(e.currentTarget.checked)} />
      {enabled && (
        <>
          <Group align="flex-start">
            <TextInput
              label="Cron expression"
              description="Server time, e.g. 0 3 * * * for 3:00 every day"
              placeholder="0 3 * * *"
              required
              value={cron}
              onChange={(e) => setCron(e.currentTarget.value)}
              error={!cron.trim() ? 'Required' : undefined}
              w={300}
            />
            <NumberInput label="Keep" description="Newest backups" min={1} value={keep} onChange={(v) => setKeep(Number(v) || 1)} w={140} />
          </Group>
          <PartSwitches value={choice} onChange={setChoice} />
        </>
      )}
    </Section>
  );
}
