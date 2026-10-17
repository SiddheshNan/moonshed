import { useEffect, useState } from 'react';
import { Anchor, Badge, Button, Card, Code, Group, MultiSelect, Select, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconPlayerRecord, IconPlayerStop, IconSearch } from '@tabler/icons-react';
import { useInfiniteQuery, useMutation, useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api, queryClient } from '../api';
import { PageHeader } from '../components/PageHeader';
import { formatBytes, formatDateTime } from '../format';
import type { App, RecordingStatus, RequestEntry } from '../types';

const pageSize = 100;

function statusColor(status: number) {
  if (status >= 500) return 'red';
  if (status >= 400) return 'orange';
  if (status >= 300) return 'blue';
  return 'green';
}

export function RequestsPage() {
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps') });
  const recording = useQuery({
    queryKey: ['recording'],
    queryFn: () => api.get<RecordingStatus>('/requests/recording'),
    refetchInterval: 5000,
  });
  const [selected, setSelected] = useState<string[]>([]);
  const recordedKey = recording.data?.app_ids.join(',');
  useEffect(() => {
    if (recordedKey !== undefined) setSelected(recordedKey ? recordedKey.split(',') : []);
  }, [recordedKey]);

  const [app, setApp] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [path, setPath] = useState('');

  const setRecording = useMutation({
    mutationFn: (ids: string[]) => api.put<RecordingStatus>('/requests/recording', { app_ids: ids }),
    onSuccess: (res) => {
      queryClient.setQueryData(['recording'], res);
      queryClient.invalidateQueries({ queryKey: ['requests'] });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  const requests = useInfiniteQuery({
    queryKey: ['requests', app, status, path],
    queryFn: ({ pageParam }) => {
      const q = new URLSearchParams({ limit: String(pageSize) });
      if (app) q.set('app', app);
      if (status) q.set('status', status);
      if (path) q.set('path', path);
      if (pageParam) q.set('before', String(pageParam));
      return api.get<RequestEntry[]>(`/requests?${q}`);
    },
    initialPageParam: 0,
    getNextPageParam: (last) => (last.length === pageSize ? last[last.length - 1].id : undefined),
    refetchInterval: recording.data?.active ? 3000 : false,
  });
  const rows = requests.data?.pages.flat() ?? [];
  const appName = new Map(apps.data?.map((a) => [a.id, a.name]));
  const appOptions = (apps.data ?? []).map((a) => ({ value: a.id, label: a.name }));
  const active = recording.data?.active ?? false;
  const proxyOff = recording.data ? !recording.data.proxy : false;
  const showList = active || rows.length > 0 || !!app || !!status || !!path;
  const changed = active && selected.slice().sort().join(',') !== recording.data!.app_ids.slice().sort().join(',');

  return (
    <Stack gap="lg">
      <PageHeader title="Requests" description="Record requests that pass through the reverse proxy, to debug an app." />

      <Card padding="lg">
        <Stack gap="sm">
          <Group justify="space-between" align="flex-start">
            <div>
              <Group gap="xs">
                <Text fw={600}>Recording</Text>
                <Badge color={active ? 'red' : 'gray'}>{active ? 'Recording' : 'Off'}</Badge>
              </Group>
              <Text size="sm" c="dimmed">
                Only the apps you pick are recorded, up to the last {recording.data?.cap.toLocaleString() ?? '100,000'} requests each. Turning
                recording off deletes everything recorded.
              </Text>
            </div>
            {active ? (
              <Button
                color="red"
                variant="light"
                leftSection={<IconPlayerStop size={16} />}
                onClick={() =>
                  modals.openConfirmModal({
                    title: 'Stop recording?',
                    children: <Text size="sm">All recorded requests will be deleted.</Text>,
                    labels: { confirm: 'Stop and delete', cancel: 'Cancel' },
                    confirmProps: { color: 'red' },
                    onConfirm: () => setRecording.mutate([]),
                  })
                }
              >
                Stop
              </Button>
            ) : (
              <Button
                leftSection={<IconPlayerRecord size={16} />}
                disabled={selected.length === 0 || proxyOff}
                loading={setRecording.isPending}
                onClick={() => setRecording.mutate(selected)}
              >
                Start recording
              </Button>
            )}
          </Group>
          {proxyOff && (
            <Text size="sm" c="dimmed">
              The reverse proxy is off, so there is nothing to record.{' '}
              <Anchor component={Link} to="/settings" size="sm">
                Turn it on
              </Anchor>{' '}
              first.
            </Text>
          )}
          <Group align="flex-end">
            <MultiSelect
              label="Apps to record"
              placeholder="Pick apps"
              data={appOptions}
              value={selected}
              onChange={setSelected}
              searchable
              style={{ flex: 1 }}
            />
            {changed && (
              <Button variant="default" onClick={() => setRecording.mutate(selected)} loading={setRecording.isPending}>
                Update apps
              </Button>
            )}
          </Group>
          {active && (
            <Group gap="lg">
              {recording.data!.app_ids.map((id) => (
                <Text key={id} size="sm" c="dimmed">
                  {appName.get(id)}: {(recording.data!.counts[id] ?? 0).toLocaleString()} recorded
                </Text>
              ))}
            </Group>
          )}
        </Stack>
      </Card>

      {showList && (
        <>
          <Group>
            <Select placeholder="All apps" data={appOptions} value={app} onChange={setApp} clearable w={200} />
            <Select
              placeholder="Any status"
              data={['2xx', '3xx', '4xx', '5xx']}
              value={status}
              onChange={setStatus}
              clearable
              w={150}
            />
            <TextInput placeholder="Path contains…" leftSection={<IconSearch size={16} />} value={path} onChange={(e) => setPath(e.currentTarget.value)} style={{ flex: 1 }} />
          </Group>

          <Card padding={0}>
            <Table.ScrollContainer minWidth={860}>
              <Table highlightOnHover fz="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Time</Table.Th>
                    <Table.Th>App</Table.Th>
                    <Table.Th>Request</Table.Th>
                    <Table.Th>Status</Table.Th>
                    <Table.Th>Duration</Table.Th>
                    <Table.Th>Size</Table.Th>
                    <Table.Th>Client</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {rows.map((r) => (
                    <Table.Tr key={r.id}>
                      <Table.Td c="dimmed" style={{ whiteSpace: 'nowrap' }}>
                        {formatDateTime(new Date(r.time).toISOString())}
                      </Table.Td>
                      <Table.Td>{appName.get(r.app_id) ?? r.app_id}</Table.Td>
                      <Table.Td maw={420}>
                        <Group gap={6} wrap="nowrap">
                          <Code>{r.method}</Code>
                          <Tooltip label={`${r.host}${r.path}${r.query ? `?${r.query}` : ''}`} multiline maw={500}>
                            <Text size="sm" truncate>
                              {r.path}
                              {r.query && <Text span c="dimmed">?{r.query}</Text>}
                            </Text>
                          </Tooltip>
                        </Group>
                      </Table.Td>
                      <Table.Td>
                        <Badge color={statusColor(r.status)}>{r.status}</Badge>
                      </Table.Td>
                      <Table.Td>{r.duration_ms < 1 ? '<1 ms' : `${r.duration_ms.toFixed(0)} ms`}</Table.Td>
                      <Table.Td>{formatBytes(r.bytes)}</Table.Td>
                      <Table.Td>
                        <Tooltip label={r.user_agent || 'no user agent'} multiline maw={400}>
                          <Text size="sm" c="dimmed">
                            {r.ip}
                          </Text>
                        </Tooltip>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </Table.ScrollContainer>
            {requests.data && rows.length === 0 && (
              <Text c="dimmed" size="sm" p="lg">
                No requests recorded yet.
              </Text>
            )}
          </Card>
          {requests.hasNextPage && (
            <Group justify="center">
              <Button variant="default" onClick={() => requests.fetchNextPage()} loading={requests.isFetchingNextPage}>
                Load more
              </Button>
            </Group>
          )}
        </>
      )}
    </Stack>
  );
}
