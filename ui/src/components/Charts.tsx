import { AreaChart } from '@mantine/charts';
import { Card, Group, Progress, SegmentedControl, Stack, Text, Tooltip } from '@mantine/core';

import { formatDateTime } from '../format';
import type { Point, Series } from '../types';

export const ranges = ['1m', '5m', '15m', '30m', '1h', '6h', '12h', '24h', '48h', '7d'].map((r) => ({ value: r, label: r }));

export function RangePicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return <SegmentedControl size="xs" data={ranges} value={value} onChange={onChange} />;
}

type ChartSeries = { key: string; label: string; color: string };

function timeLabel(t: number, range: string): string {
  const d = new Date(t * 1000);
  if (['1m', '5m', '15m'].includes(range)) {
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }
  const time = d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  return ['48h', '7d'].includes(range) ? `${d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} ${time}` : time;
}

// merge lines up the series by timestamp. Gauges also carry their bucket peak as "<key>_max".
function merge(series: Series, keys: ChartSeries[], range: string) {
  const rows = new Map<number, Record<string, number | string>>();
  for (const { key } of keys) {
    for (const p of series[key] ?? ([] as Point[])) {
      const row = rows.get(p.t) ?? { t: p.t };
      row[key] = p.v;
      if (p.max !== undefined) row[`${key}_max`] = p.max;
      rows.set(p.t, row);
    }
  }
  return [...rows.values()]
    .sort((a, b) => (a.t as number) - (b.t as number))
    .map((row) => ({ ...row, time: timeLabel(row.t as number, range) }));
}

export function MetricChart({
  title,
  series,
  keys,
  format,
  range,
  peak = false,
  value,
  detail,
  axisFormat,
}: {
  title: string;
  series: Series | undefined;
  keys: ChartSeries[];
  format: (v: number) => string;
  range: string;
  peak?: boolean;
  value?: string;
  detail?: string;
  axisFormat?: (v: number) => string;
}) {
  const data = series ? merge(series, keys, range) : [];
  const lines = keys.map((k) => ({ name: k.key, label: k.label, color: k.color }));
  if (peak) {
    for (const k of keys) {
      lines.push({ name: `${k.key}_max`, label: `${k.label} peak`, color: k.color, strokeDasharray: '4 4' } as (typeof lines)[number]);
    }
  }
  return (
    <Card padding="lg">
      {value === undefined ? (
        <Text size="sm" fw={600} mb="md">
          {title}
        </Text>
      ) : (
        <Group justify="space-between" align="flex-start" mb="md" wrap="nowrap">
          <div>
            <Text size="xs" c="dimmed" fw={500}>
              {title}
            </Text>
            <Text className="stat-value" fz={20} fw={600} mt={2} lh={1.25}>
              {value}
            </Text>
          </div>
          {detail && (
            <Text size="xs" c="dimmed" ta="right">
              {detail}
            </Text>
          )}
        </Group>
      )}
      {data.length === 0 ? (
        <Text size="sm" c="dimmed" h={180} ta="center" pt={70}>
          No data for this period
        </Text>
      ) : (
        <AreaChart
          h={180}
          data={data}
          dataKey="time"
          series={lines}
          curveType="monotone"
          withDots={false}
          strokeWidth={1.5}
          fillOpacity={0.12}
          areaProps={(s) => (s.name.endsWith('_max') ? { fillOpacity: 0, strokeOpacity: 0.55 } : {})}
          gridColor="var(--app-border)"
          textColor="var(--mantine-color-dimmed)"
          valueFormatter={format}
          yAxisProps={{ width: 76, tickFormatter: axisFormat ?? format }}
          xAxisProps={{ minTickGap: 40 }}
          gridAxis="y"
          withLegend={keys.length > 1}
          connectNulls={false}
        />
      )}
    </Card>
  );
}

export function StatCard({ label, value, detail, percent }: { label: string; value: string; detail?: string; percent?: number }) {
  return (
    <Card padding="md">
      <Text size="xs" c="dimmed" fw={500}>
        {label}
      </Text>
      <Text className="stat-value" fz={20} fw={600} mt={2} lh={1.25}>
        {value}
      </Text>
      {detail && (
        <Text size="xs" c="dimmed" mt={2}>
          {detail}
        </Text>
      )}
      {percent !== undefined && (
        <Progress value={percent} mt="sm" size={4} color={percent > 90 ? 'red' : percent > 75 ? 'orange' : 'indigo'} />
      )}
    </Card>
  );
}

export function UptimeBars({ bars, start, span }: { bars: (number | null)[]; start: string; span: number }) {
  const from = new Date(start).getTime();
  const fmt = (t: number) => formatDateTime(new Date(t).toISOString());
  return (
    <Stack gap={4}>
      <Group gap={3} wrap="nowrap">
        {bars.map((v, i) => {
          const at = from + i * span * 1000;
          const color =
            v === null ? 'var(--mantine-color-default-border)' : v >= 99.9 ? 'green.6' : v >= 95 ? 'orange.5' : 'red.5';
          const label = v === null ? 'No data' : `${v.toFixed(2)}% of checks passed`;
          return (
            <Tooltip key={i} label={`${fmt(at)} · ${label}`}>
              <Text component="div" h={32} bg={color} style={{ flex: 1, borderRadius: 3 }} />
            </Tooltip>
          );
        })}
      </Group>
      <Group justify="space-between">
        <Text size="xs" c="dimmed">
          {fmt(from)}
        </Text>
        <Text size="xs" c="dimmed">
          Now
        </Text>
      </Group>
    </Stack>
  );
}
