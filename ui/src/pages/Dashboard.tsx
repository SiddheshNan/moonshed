import { useState } from 'react';
import { Anchor, Button, Card, Group, SimpleGrid, Stack, Table, Text } from '@mantine/core';
import { IconPlus } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router';

import { api } from '../api';
import { MetricChart, RangePicker } from '../components/Charts';
import { PageHeader } from '../components/PageHeader';
import { AppStatusBadge, DeployStatusBadge } from '../components/StatusBadge';
import { TimeAgo } from '../components/TimeAgo';
import { elapsed, formatBytes, formatBytesShort, formatDuration, formatPercent } from '../format';
import { appPath } from '../paths';
import { byProblem } from '../problems';
import type { App, DeploymentRow, Project, ServerInfo, Series } from '../types';
import { NewAppModal } from './NewAppModal';

const appLimit = 6;

export function Dashboard() {
  const [range, setRange] = useState('1h');
  const [creating, setCreating] = useState(false);
  const navigate = useNavigate();
  const server = useQuery({ queryKey: ['server'], queryFn: () => api.get<ServerInfo>('/server'), refetchInterval: 5000 });
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps'), refetchInterval: 5000 });
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects') });
  const deployments = useQuery({
    queryKey: ['all-deployments', 'dashboard'],
    queryFn: () => api.get<DeploymentRow[]>('/deployments?limit=5'),
    refetchInterval: 5000,
  });
  const metrics = useQuery({
    queryKey: ['server-metrics', range],
    queryFn: () => api.get<Series>(`/server/metrics?range=${range}`),
    refetchInterval: 5000,
  });

  const all = apps.data ?? [];
  const projectName = new Map(projects.data?.map((p) => [p.id, p.name]));
  // Apps with a problem come first, so they're never cut off by the limit.
  const rows = byProblem(all);
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
          {apps.data?.length === 0 && (
            <Text c="dimmed" size="sm" px="lg" pb="lg">
              No apps yet.
            </Text>
          )}
          {rows.length > 0 && (
            <Table highlightOnHover>
              <Table.Tbody>
                {rows.slice(0, appLimit).map(({ app, problem }) => (
                  <Table.Tr key={app.id} className="row-link" onClick={() => navigate(appPath(app.project_id, app.id))}>
                    <Table.Td maw={0} w="100%">
                      <Text size="sm" fw={500} truncate>
                        {app.name}
                      </Text>
                      <Text size="xs" c={problem ? 'red' : 'dimmed'} truncate title={problem ?? undefined}>
                        {problem ?? projectName.get(app.project_id)}
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
          {rows.length > appLimit && (
            <Anchor component={Link} to="/apps" size="sm" display="block" px="lg" py="sm">
              View all {rows.length} apps
            </Anchor>
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
                  <Table.Tr key={d.id} className="row-link" onClick={() => navigate(appPath(d.project_id, d.app_id, 'deployments'))}>
                    <Table.Td fw={500} className="nowrap">
                      {d.app_name}
                    </Table.Td>
                    <Table.Td>
                      <DeployStatusBadge status={d.status} />
                    </Table.Td>
                    <Table.Td c="dimmed" className="nowrap">
                      <TimeAgo iso={d.created_at} />
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
