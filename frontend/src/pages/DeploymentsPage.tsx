import { useState } from 'react';
import { ActionIcon, Anchor, Button, Card, Code, Drawer, Group, SegmentedControl, Stack, Table, Text, Tooltip } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { IconFileText, IconPlayerStop } from '@tabler/icons-react';
import { useInfiniteQuery, useMutation } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api, queryClient } from '../api';
import { LogViewer } from '../components/LogViewer';
import { PageHeader } from '../components/PageHeader';
import { DeployStatusBadge, isActive } from '../components/StatusBadge';
import { elapsed, formatDateTime, summarizeError, timeAgo } from '../format';
import type { DeploymentRow } from '../types';

const filters = [
  { value: '', label: 'All' },
  { value: 'building', label: 'Building' },
  { value: 'queued', label: 'Queued' },
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
      <PageHeader title="Deployments" description="Every deployment across all apps, newest first." />
      <SegmentedControl w="fit-content" data={filters} value={status} onChange={setStatus} />
      <Card padding={0}>
        <Table.ScrollContainer minWidth={760}>
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>App</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>Trigger</Table.Th>
                <Table.Th>Commit</Table.Th>
                <Table.Th>Started</Table.Th>
                <Table.Th>Duration</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {rows.map((d) => (
                <Table.Tr key={d.id}>
                  <Table.Td>
                    <Anchor component={Link} to={`/apps/${d.app_id}/deployments`} fw={600} size="sm">
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
                  <Table.Td>{d.trigger}</Table.Td>
                  <Table.Td>
                    {d.git_commit ? (
                      <Tooltip label={d.git_message || d.git_commit} disabled={!d.git_message}>
                        <Code>{d.git_commit.slice(0, 7)}</Code>
                      </Tooltip>
                    ) : (
                      '—'
                    )}
                  </Table.Td>
                  <Table.Td>{timeAgo(d.created_at)}</Table.Td>
                  <Table.Td>{elapsed(d.started_at, d.finished_at)}</Table.Td>
                  <Table.Td>
                    <Group gap={4} justify="flex-end" wrap="nowrap">
                      {isActive(d.status) && (
                        <Tooltip label="Kill">
                          <ActionIcon color="red" onClick={() => kill.mutate(d.id)}>
                            <IconPlayerStop size={16} />
                          </ActionIcon>
                        </Tooltip>
                      )}
                      <Tooltip label="View log">
                        <ActionIcon onClick={() => setViewing(d)}>
                          <IconFileText size={16} />
                        </ActionIcon>
                      </Tooltip>
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
