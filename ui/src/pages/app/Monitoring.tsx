import { useState } from 'react';
import { Badge, Card, Group, SegmentedControl, SimpleGrid, Stack, Table, Text } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';

import { api } from '../../api';
import { MetricChart, RangePicker, StatCard, UptimeBars } from '../../components/Charts';
import { appLook } from '../../components/StatusBadge';
import { elapsed, formatBytes, formatBytesShort, formatDateTime, formatPercent, formatSmallPercent } from '../../format';
import type { App, AppMetrics, Health, Uptime } from '../../types';
import { HealthCheck } from './HealthCheck';

const healthLook: Record<Health, { color: string; label: string }> = {
  up: { color: 'green', label: 'Up' },
  down: { color: 'red', label: 'Down' },
  unknown: { color: 'gray', label: 'Not checked yet' },
};

function uptimeValue(v: number | null) {
  return v === null ? 'No data' : formatPercent(v, 2);
}

export function Monitoring({ app }: { app: App }) {
  const [range, setRange] = useState('1h');
  const [uptimeRange, setUptimeRange] = useState('24h');
  const metrics = useQuery({
    queryKey: ['app-metrics', app.id, range],
    queryFn: () => api.get<AppMetrics>(`/apps/${app.id}/metrics?range=${range}`),
    refetchInterval: 5000,
  });
  const uptime = useQuery({
    queryKey: ['uptime', app.id, uptimeRange],
    queryFn: () => api.get<Uptime>(`/apps/${app.id}/uptime?range=${uptimeRange}`),
    refetchInterval: 15000,
  });
  const u = uptime.data;
  const badge = app.status.state === 'running' ? u && healthLook[u.health] : appLook(app);

  return (
    <Stack gap="lg">
      <Card>
        <Group justify="space-between" mb="md">
          <Group gap="sm">
            <Text fw={600}>Uptime</Text>
            {badge && <Badge color={badge.color}>{badge.label}</Badge>}
          </Group>
          <SegmentedControl
            size="xs"
            value={uptimeRange}
            onChange={setUptimeRange}
            data={[
              { value: '24h', label: '24h' },
              { value: '7d', label: '7d' },
            ]}
          />
        </Group>
        {u && (
          <Stack>
            <SimpleGrid cols={2}>
              <StatCard label="24 hours" value={uptimeValue(u.uptime_24h)} />
              <StatCard label="7 days" value={uptimeValue(u.uptime_7d)} />
            </SimpleGrid>
            <UptimeBars bars={u.bars} start={u.bars_start} span={u.bar_span} />
            <Text size="xs" c="dimmed">
              {app.health_check ? `Share of passed health checks (${app.health_check}).` : 'Share of checks that found the process running.'}
            </Text>
            {u.incidents.length > 0 && (
              <Table.ScrollContainer minWidth={520}>
                <Table verticalSpacing="xs">
                  <Table.Thead>
                    <Table.Tr>
                      <Table.Th>Went down</Table.Th>
                      <Table.Th>Duration</Table.Th>
                      <Table.Th>Reason</Table.Th>
                    </Table.Tr>
                  </Table.Thead>
                  <Table.Tbody>
                    {u.incidents.map((i) => (
                      <Table.Tr key={i.id}>
                        <Table.Td>{formatDateTime(i.down_at)}</Table.Td>
                        <Table.Td>{i.up_at ? elapsed(i.down_at, i.up_at) : <Badge color="red">ongoing</Badge>}</Table.Td>
                        <Table.Td>
                          <Text size="sm" c="dimmed" lineClamp={2}>
                            {i.reason}
                          </Text>
                        </Table.Td>
                      </Table.Tr>
                    ))}
                  </Table.Tbody>
                </Table>
              </Table.ScrollContainer>
            )}
          </Stack>
        )}
      </Card>

      <HealthCheck key={`${JSON.stringify(app.config)}-${app.current_release_id}`} app={app} />

      <Group justify="space-between">
        <Text fw={600}>Resources</Text>
        <RangePicker value={range} onChange={setRange} />
      </Group>
      <SimpleGrid cols={{ base: 1, md: 2 }}>
        <MetricChart
          title="CPU (% of server)"
          range={range}
          series={metrics.data?.app}
          peak
          keys={[{ key: 'cpu', label: 'CPU', color: 'indigo.5' }]}
          format={formatSmallPercent}
        />
        <MetricChart
          title="Memory"
          range={range}
          series={metrics.data?.app}
          peak
          keys={[{ key: 'mem', label: 'Memory', color: 'violet.5' }]}
          format={formatBytes}
          axisFormat={formatBytesShort}
        />
        {app.health_check && (
          <MetricChart
            title="Health check response time"
            range={range}
            series={metrics.data?.app}
            keys={[{ key: 'health_ms', label: 'Response', color: 'cyan.5' }]}
            format={(v) => `${v.toFixed(1)} ms`}
          />
        )}
      </SimpleGrid>

      {Object.entries(metrics.data?.traffic ?? {}).map(([host, series]) => (
        <Stack key={host} gap="sm">
          <Text fw={600}>Traffic · {host}</Text>
          <SimpleGrid cols={{ base: 1, md: 2 }}>
            <MetricChart
              title="Requests per second"
              range={range}
              series={series}
              keys={[
                { key: 'requests', label: 'All', color: 'indigo.5' },
                { key: '4xx', label: '4xx', color: 'orange.5' },
                { key: '5xx', label: '5xx', color: 'red.5' },
              ]}
              format={(v) => v.toFixed(2)}
            />
            <MetricChart
              title="Average response time"
              range={range}
              series={series}
              keys={[{ key: 'latency', label: 'Latency', color: 'violet.5' }]}
              format={(v) => `${v.toFixed(0)} ms`}
            />
          </SimpleGrid>
        </Stack>
      ))}
    </Stack>
  );
}
