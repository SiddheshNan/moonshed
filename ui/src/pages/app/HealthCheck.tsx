import { useState } from 'react';
import { Group, NumberInput, SegmentedControl, SimpleGrid, Stack, Switch, Text, TextInput } from '@mantine/core';

import { duration, mono, Origin, seconds } from '../../components/Origin';
import type { App, HealthCheck as Check, Overrides } from '../../types';
import { Section, useSave } from './Settings';

type Kind = 'process' | 'http' | 'tcp';

// HealthCheck edits how the app's health is checked and whether a failing app is restarted.
// Changes apply on the next check, without a deploy.
export function HealthCheck({ app }: { app: App }) {
  const save = useSave(app);
  const file = app.file;
  const [health, setHealth] = useState<Check | undefined>(app.config?.health);
  const [restart, setRestart] = useState<boolean | undefined>(app.config?.restart_unhealthy);

  const current: Check = health ?? file?.health ?? {};
  const kind: Kind = current.http ? 'http' : current.tcp ? 'tcp' : 'process';
  const set = (p: Partial<Check>) => setHealth({ ...current, ...p });
  const restartOn = restart ?? file?.restart_unhealthy ?? false;

  const config: Overrides = { ...app.config, health, restart_unhealthy: restart };
  const changed = JSON.stringify(health) !== JSON.stringify(app.config?.health) || restart !== app.config?.restart_unhealthy;
  const num = (v: string | number) => (v === '' ? undefined : Number(v));

  return (
    <Section title="Health check" onSave={changed ? () => save.mutate({ config }) : undefined} saving={save.isPending}>
      <Group justify="space-between" align="flex-end">
        <SegmentedControl
          value={kind}
          onChange={(v) => set({ http: v === 'http' ? current.http || '/' : undefined, tcp: v === 'tcp' || undefined })}
          data={[
            { value: 'process', label: 'Process only' },
            { value: 'http', label: 'HTTP' },
            { value: 'tcp', label: 'TCP' },
          ]}
        />
        <Origin changed={health !== undefined} inFile={!!file?.health} onReset={() => setHealth(undefined)} />
      </Group>
      <Text size="xs" c="dimmed">
        {kind === 'process' && 'Only checks that the process is running. An HTTP check also catches an app that runs but no longer responds.'}
        {kind === 'http' && `Up while GET http://127.0.0.1:${app.port || '<port>'}${current.http ?? '/'} answers below 400.`}
        {kind === 'tcp' && `Up while port ${app.port || '<port>'} accepts connections.`}
      </Text>
      {kind === 'http' && (
        <TextInput label="Path" styles={mono} placeholder="/healthz" value={current.http ?? ''} onChange={(e) => set({ http: e.currentTarget.value })} />
      )}
      {kind !== 'process' && (
        <SimpleGrid cols={{ base: 2, sm: 4 }}>
          <NumberInput
            label="Check every"
            suffix=" s"
            min={1}
            placeholder="30"
            value={seconds(current.interval) ?? ''}
            onChange={(v) => set({ interval: num(v) === undefined ? undefined : duration(num(v)!) })}
          />
          <NumberInput
            label="Timeout"
            suffix=" s"
            min={1}
            placeholder="5"
            value={seconds(current.timeout) ?? ''}
            onChange={(v) => set({ timeout: num(v) === undefined ? undefined : duration(num(v)!) })}
          />
          <NumberInput
            label="Down after"
            suffix=" failed checks"
            min={1}
            placeholder="3"
            value={current.failures ?? ''}
            onChange={(v) => set({ failures: num(v) })}
          />
          <NumberInput
            label="Wait after start"
            suffix=" s"
            min={0}
            placeholder="10"
            value={seconds(current.grace) ?? ''}
            onChange={(v) => set({ grace: num(v) === undefined ? undefined : duration(num(v)!) })}
          />
        </SimpleGrid>
      )}
      <Stack gap={4}>
        <Group justify="space-between" wrap="nowrap">
          <Switch
            label="Restart the app when it's down"
            checked={restartOn && kind !== 'process'}
            disabled={kind === 'process'}
            onChange={(e) => setRestart(e.currentTarget.checked)}
          />
          {kind !== 'process' && (
            <Origin changed={restart !== undefined} inFile={file?.restart_unhealthy !== undefined} onReset={() => setRestart(undefined)} />
          )}
        </Group>
        {kind === 'process' && (
          <Text size="xs" c="dimmed">
            A crashed process is restarted anyway. This is for apps that keep running but fail their check.
          </Text>
        )}
      </Stack>
    </Section>
  );
}
