import { useState } from 'react';
import { Anchor, Button, Card, Group, SimpleGrid, Stack, Table, Text } from '@mantine/core';
import { IconCircleCheck, IconPlus } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router';

import { api } from '../api';
import { MetricChart, RangePicker } from '../components/Charts';
import { PageHeader } from '../components/PageHeader';
import { AppStatusBadge, DeployStatusBadge } from '../components/StatusBadge';
import { elapsed, formatBytes, formatBytesShort, formatDuration, formatPercent, summarizeError, timeAgo } from '../format';
import type { App, DeploymentRow, ServerInfo, Series } from '../types';
import { NewAppModal } from './NewAppModal';

// problem explains what is wrong with an app, or returns null when nothing is.
function problem(app: App, lastDeploy: DeploymentRow | undefined): string | null {
  if (app.status.state === 'crashed') {
    return app.status.restarts > 0 ? `Crashed, restarted ${app.status.restarts} times` : 'Crashed';
  }
  if (app.status.state === 'running' && app.health === 'down') return 'Failing its health check';
  if (app.status.state !== 'running' && app.desired_state === 'running' && app.current_release_id) return 'Stopped unexpectedly';
  if (lastDeploy?.status === 'failed') return `Last deployment failed${lastDeploy.error ? `: ${summarizeError(lastDeploy.error)}` : ''}`;
  return null;
}

export function Dashboard() {
  const [range, setRange] = useState('1h');
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();
  const server = useQuery({ queryKey: ['server'], queryFn: () => api.get<ServerInfo>('/server'), refetchInterval: 5000 });
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps'), refetchInterval: 5000 });
  const deployments = useQuery({
    queryKey: ['all-deployments', 'dashboard'],
    queryFn: () => api.get<DeploymentRow[]>('/deployments?limit=100'),
    refetchInterval: 5000,
  });
  const metrics = useQuery({
    queryKey: ['server-metrics', range],
    queryFn: () => api.get<Series>(`/server/metrics?range=${range}`),
    refetchInterval: 5000,
  });

  const latest = new Map<string, DeploymentRow>();
  for (const d of deployments.data ?? []) {
    if (!latest.has(d.app_id)) latest.set(d.app_id, d);
  }
  const all = apps.data ?? [];
  const problems = all.flatMap((app) => {
    const reason = problem(app, latest.get(app.id));
    return reason ? [{ app, reason }] : [];
  });
  const running = all.filter((a) => a.status.state === 'running').length;
  const s = server.data?.stats;

  return (
    <Stack gap="lg">
      <PageHeader
        title="Dashboard"
        description={
          s && (
            <>
              {s.hostname} · {s.os} · {s.cores} cores · up {formatDuration(s.uptime)}
              {server.data?.docker ? ` · Docker ${server.data.docker_version}` : ' · Docker not available'}
            </>
          )
        }
        actions={
          <Button leftSection={<IconPlus size={16} />} onClick={() => setCreating(true)}>
            New app
          </Button>
        }
      />

      <SimpleGrid cols={{ base: 1, lg: 2 }}>
        <Card padding={0}>
          <Group justify="space-between" px="lg" py="md">
            <Text fw={600}>Apps</Text>
            <Text size="sm" c="dimmed">
              {all.length} total · {running} running
            </Text>
          </Group>
          {apps.data && problems.length === 0 && (
            <Group gap="xs" px="lg" pb="lg">
              <IconCircleCheck size={18} color="var(--mantine-color-green-6)" />
              <Text size="sm" c="dimmed">
                {all.length === 0 ? 'No apps yet.' : 'Nothing needs attention.'}
              </Text>
            </Group>
          )}
          {problems.length > 0 && (
            <Table highlightOnHover>
              <Table.Tbody>
                {problems.map(({ app, reason }) => (
                  <Table.Tr key={app.id} className="row-link" onClick={() => navigate(`/apps/${app.id}`)}>
                    <Table.Td maw={0} w="100%">
                      <Text size="sm" fw={500} truncate>
                        {app.name}
                      </Text>
                      <Text size="xs" c="dimmed" truncate title={reason}>
                        {reason}
                      </Text>
                    </Table.Td>
                    <Table.Td ta="right">
                      <AppStatusBadge app={app} />
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          )}
        </Card>

        <Card padding={0}>
          <Group justify="space-between" px="lg" py="md">
            <Text fw={600}>Recent deployments</Text>
            <Anchor component={Link} to="/deployments" size="sm">
              View all
            </Anchor>
          </Group>
          {deployments.data?.length === 0 ? (
            <Text c="dimmed" size="sm" px="lg" pb="lg">
              Nothing deployed yet.
            </Text>
          ) : (
            <Table highlightOnHover>
              <Table.Tbody>
                {deployments.data?.slice(0, 5).map((d) => (
                  <Table.Tr key={d.id} className="row-link" onClick={() => navigate(`/apps/${d.app_id}/deployments`)}>
                    <Table.Td fw={500} className="nowrap">
                      {d.app_name}
                    </Table.Td>
                    <Table.Td>
                      <DeployStatusBadge status={d.status} />
                    </Table.Td>
                    <Table.Td c="dimmed" className="nowrap">
                      {timeAgo(d.created_at)}
                    </Table.Td>
                    <Table.Td c="dimmed" ta="right" className="nowrap">
                      {elapsed(d.started_at, d.finished_at)}
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          )}
        </Card>
      </SimpleGrid>

      <Group justify="space-between">
        <Text fw={600}>Server</Text>
        <RangePicker value={range} onChange={setRange} />
      </Group>
      <SimpleGrid cols={{ base: 1, md: 2 }}>
        <MetricChart
          title="CPU"
          value={s ? formatPercent(s.cpu) : '—'}
          detail={s ? `load ${s.load1.toFixed(2)}` : undefined}
          range={range}
          series={metrics.data}
          peak
          keys={[{ key: 'cpu', label: 'CPU', color: 'indigo.5' }]}
          format={(v) => formatPercent(v)}
        />
        <MetricChart
          title="Memory"
          value={s ? formatBytes(s.mem_used) : '—'}
          detail={s ? `of ${formatBytes(s.mem_total)} · ${formatPercent((s.mem_used / s.mem_total) * 100, 0)}` : undefined}
          range={range}
          series={metrics.data}
          keys={[{ key: 'mem', label: 'Used', color: 'violet.5' }]}
          format={formatBytes}
          axisFormat={formatBytesShort}
        />
        <MetricChart
          title="Network"
          value={s ? `↓ ${formatBytes(s.net_in)}/s` : '—'}
          detail={s ? `↑ ${formatBytes(s.net_out)}/s` : undefined}
          range={range}
          series={metrics.data}
          keys={[
            { key: 'net_in', label: 'In', color: 'cyan.5' },
            { key: 'net_out', label: 'Out', color: 'indigo.5' },
          ]}
          format={(v) => `${formatBytes(v)}/s`}
          axisFormat={(v) => `${formatBytesShort(v)}/s`}
        />
        <MetricChart
          title="Disk"
          value={s ? formatBytes(s.disk_used) : '—'}
          detail={s ? `of ${formatBytes(s.disk_total)} · ${formatPercent((s.disk_used / s.disk_total) * 100, 0)}` : undefined}
          range={range}
          series={metrics.data}
          keys={[{ key: 'disk', label: 'Used', color: 'orange.5' }]}
          format={formatBytes}
          axisFormat={formatBytesShort}
        />
      </SimpleGrid>

      <NewAppModal opened={creating} onClose={() => setCreating(false)} />
    </Stack>
  );
}
