import { useState } from 'react';
import { Badge, Button, Card, Group, Select, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core';
import { IconSearch } from '@tabler/icons-react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import { formatDateTime, timeAgo } from '../../format';
import { appPath } from '../../paths';
import type { App, AuditEntry } from '../../types';

const pageSize = 50;

const areas = [
  { value: 'app.', label: 'Apps' },
  { value: 'domain.', label: 'Domains' },
  { value: 'project.', label: 'Projects' },
  { value: 'backup.', label: 'Backups' },
  { value: 'settings.', label: 'Settings' },
  { value: 'git.', label: 'Git' },
  { value: 'requests.', label: 'Requests' },
  { value: 'auth.', label: 'Logins' },
  { value: 'user.', label: 'Users' },
  { value: 'token.', label: 'API tokens' },
];

const actionLabels: Record<string, string> = {
  'app.create': 'Created app',
  'app.update': 'Changed app settings',
  'app.delete': 'Deleted app',
  'app.deploy': 'Deployed',
  'app.redeploy': 'Redeployed',
  'app.kill': 'Killed deployment',
  'app.start': 'Started app',
  'app.stop': 'Stopped app',
  'app.restart': 'Restarted app',
  'app.env': 'Changed environment',
  'domain.create': 'Added domain',
  'domain.update': 'Changed domain',
  'domain.delete': 'Removed domain',
  'domain.certificate': 'Replaced certificate',
  'project.create': 'Created project',
  'project.update': 'Renamed project',
  'project.delete': 'Deleted project',
  'backup.create': 'Backed up',
  'backup.restore': 'Restored backup',
  'backup.delete': 'Deleted backup',
  'settings.update': 'Changed settings',
  'settings.panel_certificate': 'Replaced panel certificate',
  'git.credential.create': 'Added git token',
  'git.credential.update': 'Changed git token',
  'git.credential.delete': 'Removed git token',
  'git.github.create': 'Created GitHub App',
  'git.github.install': 'Installed GitHub App',
  'git.github.delete': 'Removed GitHub App',
  'requests.record': 'Started recording requests',
  'requests.stop': 'Stopped recording requests',
  'auth.login': 'Signed in',
  'auth.login_failed': 'Failed sign-in',
  'auth.logout': 'Signed out',
  'user.setup': 'Set up the first user',
  'user.create': 'Added user',
  'user.delete': 'Removed user',
  'user.password': 'Changed password',
  'token.create': 'Created API token',
  'token.delete': 'Deleted API token',
};

const actorLabels: Record<AuditEntry['actor_type'], string> = { user: 'User', token: 'API token', webhook: 'Webhook', system: 'System' };

const actorColors: Record<AuditEntry['actor_type'], string> = { user: 'gray', token: 'blue', webhook: 'grape', system: 'gray' };

export function AuditPage() {
  const [action, setAction] = useState<string | null>(null);
  const [actor, setActor] = useState('');
  const list = useInfiniteQuery({
    queryKey: ['audit', action, actor],
    queryFn: ({ pageParam }) => {
      const q = new URLSearchParams({ limit: String(pageSize) });
      if (action) q.set('action', action);
      if (actor) q.set('actor', actor);
      if (pageParam) q.set('before', String(pageParam));
      return api.get<AuditEntry[]>(`/audit?${q}`);
    },
    initialPageParam: 0,
    getNextPageParam: (last) => (last.length === pageSize ? last[last.length - 1].seq : undefined),
  });
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps') });
  const appsById = new Map(apps.data?.map((a) => [a.id, a]));
  const rows = list.data?.pages.flat() ?? [];

  return (
    <Stack gap="lg">
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Audit log" description="Kept for one year." />
      <Group>
        <Select placeholder="Everything" data={areas} value={action} onChange={setAction} clearable w={200} />
        <TextInput placeholder="Username" leftSection={<IconSearch size={16} />} value={actor} onChange={(e) => setActor(e.currentTarget.value.trim())} w={220} />
      </Group>
      <Card padding={0}>
        <Table.ScrollContainer minWidth={820}>
          <Table>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>When</Table.Th>
                <Table.Th>Who</Table.Th>
                <Table.Th>Action</Table.Th>
                <Table.Th>Target</Table.Th>
                <Table.Th>Details</Table.Th>
                <Table.Th>IP</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {rows.map((e) => {
                const target = e.target_type === 'app' ? appsById.get(e.target_id) : undefined;
                return (
                  <Table.Tr key={e.seq}>
                    <Table.Td style={{ whiteSpace: 'nowrap' }}>
                      <Tooltip label={formatDateTime(e.time)}>
                        <Text size="sm">{timeAgo(e.time)}</Text>
                      </Tooltip>
                    </Table.Td>
                    <Table.Td className="nowrap">
                      <Group gap={6} wrap="nowrap">
                        <Text size="sm" fw={500}>
                          {e.actor_name}
                        </Text>
                        {e.actor_type !== 'user' && (
                          <Badge size="xs" color={actorColors[e.actor_type]}>
                            {actorLabels[e.actor_type]}
                          </Badge>
                        )}
                      </Group>
                    </Table.Td>
                    <Table.Td className="nowrap">
                      <Text size="sm" c={e.action.endsWith('failed') ? 'red' : undefined} title={e.action}>
                        {actionLabels[e.action] ?? e.action}
                      </Text>
                    </Table.Td>
                    <Table.Td className="nowrap">
                      {target ? (
                        <Text component={Link} to={appPath(target.project_id, target.id)} size="sm" c="indigo">
                          {e.target_name}
                        </Text>
                      ) : (
                        <Text size="sm">{e.target_name || '—'}</Text>
                      )}
                    </Table.Td>
                    <Table.Td maw={360}>
                      <Text size="sm" c="dimmed" lineClamp={2}>
                        {e.details || '—'}
                      </Text>
                    </Table.Td>
                    <Table.Td c="dimmed">{e.ip}</Table.Td>
                  </Table.Tr>
                );
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        {list.data && rows.length === 0 && (
          <Text c="dimmed" size="sm" p="lg">
            Nothing recorded yet.
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
    </Stack>
  );
}
