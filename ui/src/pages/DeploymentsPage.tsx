import { useState } from 'react';
import { Anchor, Button, Card, Drawer, Group, SegmentedControl, Stack, Table, Text } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { IconFileText, IconPlayerStop } from '@tabler/icons-react';
import { useInfiniteQuery, useMutation } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api, queryClient } from '../api';
import { Commit } from '../components/Commit';
import { LogViewer } from '../components/LogViewer';
import { PageHeader } from '../components/PageHeader';
import { DeployStatusBadge, isActive } from '../components/StatusBadge';
import { elapsed, formatDateTime, summarizeError, timeAgo, triggerLabel } from '../format';
import { appPath } from '../paths';
import type { DeploymentRow } from '../types';

const filters = [
  { value: '', label: 'All' },
  { value: 'queued', label: 'Queued' },
  { value: 'building', label: 'Building' },
  { value: 'success', label: 'Succeeded' },
  { value: 'failed', label: 'Failed' },
  { value: 'killed', label: 'Killed' },
];

const pageSize = 50;

export function DeploymentsPage() {
  const [status, setStatus] = useState('');
  const [viewing, setViewing] = useState<DeploymentRow | null>(null);
  const list = useInfiniteQuery({
    queryKey: ['all-deployments', status],
    queryFn: ({ pageParam }) =>
      api.get<DeploymentRow[]>(`/deployments?limit=${pageSize}&status=${status}${pageParam ? `&before=${pageParam}` : ''}`),
    initialPageParam: '',
    getNextPageParam: (last): string | undefined => (last.length === pageSize ? last[last.length - 1].id : undefined),
    refetchInterval: (q) => (q.state.data?.pages[0]?.some((d) => isActive(d.status)) ? 2000 : 10000),
  });
  const rows = list.data?.pages.flat() ?? [];

  const kill = useMutation({
    mutationFn: (id: string) => api.post(`/deployments/${id}/kill`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['all-deployments'] }),
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Stack gap="lg">
      <PageHeader title="Deployments" />
      <SegmentedControl w="fit-content" data={filters} value={status} onChange={setStatus} />
      <Card padding={0}>
        <Table.ScrollContainer minWidth={860}>
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>App</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>Source</Table.Th>
                <Table.Th>Trigger</Table.Th>
                <Table.Th>Started</Table.Th>
                <Table.Th>Duration</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {rows.map((d) => (
                <Table.Tr key={d.id}>
                  <Table.Td className="nowrap">
                    <Anchor component={Link} to={appPath(d.project_id, d.app_id, 'deployments')} fw={600} size="sm">
                      {d.app_name}
                    </Anchor>
                  </Table.Td>
                  <Table.Td>
                    <Stack gap={2}>
                      <DeployStatusBadge status={d.status} />
                      {d.error && d.status === 'failed' && (
                        <Text size="xs" c="red" maw={300} lineClamp={2} title={d.error}>
                          {summarizeError(d.error)}
                        </Text>
                      )}
                    </Stack>
                  </Table.Td>
                  <Table.Td className="nowrap">
                    {d.git_commit ? <Commit gitUrl={d.git_url} sha={d.git_commit} message={d.git_message} /> : d.bundle_id ? 'Zip upload' : '—'}
                  </Table.Td>
                  <Table.Td className="nowrap">{triggerLabel(d.trigger)}</Table.Td>
                  <Table.Td className="nowrap" title={formatDateTime(d.created_at)}>
                    {timeAgo(d.created_at)}
                  </Table.Td>
                  <Table.Td className="nowrap">{elapsed(d.started_at, d.finished_at)}</Table.Td>
                  <Table.Td>
                    <Group gap="xs" justify="flex-end" wrap="nowrap">
                      {isActive(d.status) && (
                        <Button size="compact-sm" variant="light" color="red" leftSection={<IconPlayerStop size={14} />} onClick={() => kill.mutate(d.id)}>
                          Kill build
                        </Button>
                      )}
                      <Button size="compact-sm" variant="default" leftSection={<IconFileText size={14} />} onClick={() => setViewing(d)}>
                        View log
                      </Button>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        {list.data && rows.length === 0 && (
          <Text c="dimmed" size="sm" p="lg">
            No deployments.
          </Text>
        )}
      </Card>
      {list.hasNextPage && (
        <Group justify="center">
          <Button variant="default" onClick={() => list.fetchNextPage()} loading={list.isFetchingNextPage}>
            Load more
          </Button>
        </Group>
      )}
      <Drawer opened={viewing !== null} onClose={() => setViewing(null)} title={viewing ? `${viewing.app_name} · ${formatDateTime(viewing.created_at)}` : ''} position="right" size="xl">
        {viewing && <LogViewer url={`/api/deployments/${viewing.id}/log`} height="calc(100vh - 140px)" />}
      </Drawer>
    </Stack>
  );
}
