import { useState } from 'react';
import { Button, Card, Group, ScrollArea, Select, Stack, Text } from '@mantine/core';
import { IconDownload } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';

import { api } from '../../api';
import { ansi } from '../../components/ansi';
import { InlineSetting } from '../../components/InlineSetting';
import { LogViewer } from '../../components/LogViewer';
import type { App } from '../../types';
import { useSave } from './Settings';

export function Logs({ app }: { app: App }) {
  const [day, setDay] = useState<string | null>('live');
  const save = useSave(app);
  const days = useQuery({ queryKey: ['log-days', app.id], queryFn: () => api.get<string[]>(`/apps/${app.id}/logs/days`) });
  const history = useQuery({
    queryKey: ['logs', app.id, day],
    queryFn: async () => {
      const res = await fetch(`/api/apps/${app.id}/logs?date=${day}`);
      return res.text();
    },
    enabled: !!day && day !== 'live',
  });

  return (
    <Card>
      <Stack>
        <Group justify="space-between">
          <Select
            size="xs"
            w={200}
            value={day}
            onChange={setDay}
            allowDeselect={false}
            data={[{ value: 'live', label: 'Live tail' }, ...(days.data ?? []).map((d) => ({ value: d, label: d }))]}
          />
          {day === 'live' && (
            <InlineSetting
              label="Keep logs for (days)"
              value={app.log_retention_days}
              saving={save.isPending}
              onSave={(v, done) => save.mutate({ log_retention_days: v }, { onSuccess: done })}
            >
             Logs are kept for {app.log_retention_days} days ·
            </InlineSetting>
          )}
          {day && day !== 'live' && (
            <Button
              size="xs"
              variant="default"
              leftSection={<IconDownload size={14} />}
              component="a"
              href={`/api/apps/${app.id}/logs?date=${day}&download`}
            >
              Download
            </Button>
          )}
        </Group>
        {day === 'live' ? (
          <LogViewer url={`/api/apps/${app.id}/logs/stream`} />
        ) : (
          <ScrollArea h={560} className="terminal" p="md">
            <Text component="pre" ff="monospace" fz="xs" m={0} style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
              {history.isPending ? 'Loading…' : history.data ? ansi(history.data) : 'No logs for this day.'}
            </Text>
          </ScrollArea>
        )}
      </Stack>
    </Card>
  );
}
