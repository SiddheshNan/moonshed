import { useState } from 'react';
import { ActionIcon, Badge, Button, Card, Code, Drawer, Group, Stack, Table, Text, Tooltip } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconFileText, IconPlayerStop } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { LogViewer } from '../../components/LogViewer';
import { DeployStatusBadge, isActive } from '../../components/StatusBadge';
import { elapsed, formatDateTime, summarizeError, timeAgo } from '../../format';
import type { App, Bundle, Deployment } from '../../types';

export function Deployments({ app }: { app: App }) {
  const [viewing, setViewing] = useState<Deployment | null>(null);
  const deployments = useQuery({
    queryKey: ['deployments', app.id],
    queryFn: () => api.get<Deployment[]>(`/apps/${app.id}/deployments`),
    refetchInterval: (q) => (q.state.data?.some((d) => isActive(d.status)) ? 2000 : 10000),
  });
  const bundles = useQuery({ queryKey: ['bundles', app.id], queryFn: () => api.get<Bundle[]>(`/apps/${app.id}/bundles`) });
  const stored = new Set(bundles.data?.map((b) => b.id));

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['deployments', app.id] });
    queryClient.invalidateQueries({ queryKey: ['bundles', app.id] });
    queryClient.invalidateQueries({ queryKey: ['app', app.id] });
  };
  const kill = useMutation({
    mutationFn: (id: string) => api.post(`/deployments/${id}/kill`),
    onSuccess: refresh,
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const redeploy = useMutation({
    mutationFn: (bundleId: string) => api.post<Deployment>(`/apps/${app.id}/bundles/${bundleId}/redeploy`),
    onSuccess: (d) => {
      refresh();
      setViewing(d);
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  const confirmRedeploy = (d: Deployment) =>
    modals.openConfirmModal({
      title: 'Redeploy this version?',
      children: (
        <Text size="sm">
          Rebuilds {d.git_commit ? <Code>{d.git_commit.slice(0, 7)}</Code> : `the upload from ${formatDateTime(d.created_at)}`} and replaces the
          running version.
        </Text>
      ),
      labels: { confirm: 'Redeploy', cancel: 'Cancel' },
      onConfirm: () => redeploy.mutate(d.bundle_id),
    });

  return (
    <Stack gap="lg">
      <Card padding={0}>
        <Group justify="space-between" p="md">
          <Text fw={600}>Deployments</Text>
          <Text size="sm" c="dimmed">
            The last {app.keep_bundles} versions can be redeployed
          </Text>
        </Group>
        <Table.ScrollContainer minWidth={680}>
          <Table verticalSpacing="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Status</Table.Th>
                <Table.Th>Source</Table.Th>
                <Table.Th>Trigger</Table.Th>
                <Table.Th>Started</Table.Th>
                <Table.Th>Duration</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {deployments.data?.map((d) => {
                const live = d.status === 'success' && !!d.release_id && d.release_id === app.current_release_id;
                return (
                  <Table.Tr key={d.id}>
                    <Table.Td>
                      <Group gap="xs" wrap="nowrap">
                        <DeployStatusBadge status={d.status} />
                        {live && (
                          <Badge size="sm" variant="light" color="green">
                            live
                          </Badge>
                        )}
                      </Group>
                      {d.error && d.status === 'failed' && (
                        <Text size="xs" c="red" maw={360} lineClamp={2} mt={4} title={d.error}>
                          {summarizeError(d.error)}
                        </Text>
                      )}
                    </Table.Td>
                    <Table.Td>
                      {d.git_commit ? (
                        <Tooltip label={d.git_message || d.git_commit} disabled={!d.git_message}>
                          <Code>{d.git_commit.slice(0, 7)}</Code>
                        </Tooltip>
                      ) : d.bundle_id ? (
                        'upload'
                      ) : (
                        '—'
                      )}
                    </Table.Td>
                    <Table.Td>{d.trigger}</Table.Td>
                    <Table.Td className="nowrap" title={formatDateTime(d.created_at)}>
                      {timeAgo(d.created_at)}
                    </Table.Td>
                    <Table.Td>{elapsed(d.started_at, d.finished_at)}</Table.Td>
                    <Table.Td>
                      <Group gap={4} justify="flex-end" wrap="nowrap">
                        {d.status === 'success' && !live && stored.has(d.bundle_id) && (
                          <Button size="compact-xs" variant="default" onClick={() => confirmRedeploy(d)}>
                            Redeploy
                          </Button>
                        )}
                        {isActive(d.status) && (
                          <Tooltip label="Kill">
                            <ActionIcon variant="subtle" color="red" onClick={() => kill.mutate(d.id)}>
                              <IconPlayerStop size={16} />
                            </ActionIcon>
                          </Tooltip>
                        )}
                        <Tooltip label="View log">
                          <ActionIcon variant="subtle" color="gray" onClick={() => setViewing(d)}>
                            <IconFileText size={16} />
                          </ActionIcon>
                        </Tooltip>
                      </Group>
                    </Table.Td>
                  </Table.Tr>
                );
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        {deployments.data?.length === 0 && (
          <Text c="dimmed" size="sm" px="md" pb="md">
            No deployments yet.
          </Text>
        )}
      </Card>

      <Drawer
        opened={viewing !== null}
        onClose={() => {
          setViewing(null);
          refresh();
        }}
        title={viewing ? `Deployment · ${formatDateTime(viewing.created_at)}` : ''}
        position="right"
        size="xl"
      >
        {viewing && <LogViewer url={`/api/deployments/${viewing.id}/log`} height="calc(100vh - 140px)" />}
      </Drawer>
    </Stack>
  );
}
