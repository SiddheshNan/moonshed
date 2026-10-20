import { useEffect, useState } from 'react';
import { Alert, Button, Card, Code, Group, Stack, Text, Textarea } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import type { App } from '../../types';

function toDotenv(env: Record<string, string>): string {
  return Object.entries(env)
    .map(([k, v]) => `${k}=${/[\s#"']/.test(v) || v.includes('\n') ? JSON.stringify(v) : v}`)
    .join('\n');
}

function parseDotenv(text: string): Record<string, string> {
  const env: Record<string, string> = {};
  for (const raw of text.split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#')) continue;
    const eq = line.indexOf('=');
    if (eq < 1) throw new Error(`Invalid line: ${line}`);
    const key = line.slice(0, eq).trim().replace(/^export\s+/, '');
    let value = line.slice(eq + 1).trim();
    if (value.startsWith('"') && value.endsWith('"') && value.length >= 2) {
      value = JSON.parse(value);
    } else if (value.startsWith("'") && value.endsWith("'") && value.length >= 2) {
      value = value.slice(1, -1);
    }
    env[key] = value;
  }
  return env;
}

export function Environment({ app }: { app: App }) {
  const env = useQuery({ queryKey: ['env', app.id], queryFn: () => api.get<Record<string, string>>(`/apps/${app.id}/env`) });
  const [text, setText] = useState('');
  const [parseError, setParseError] = useState('');

  useEffect(() => {
    if (env.data) setText(toDotenv(env.data));
  }, [env.data]);

  const save = useMutation({
    mutationFn: (vars: Record<string, string>) => api.put(`/apps/${app.id}/env`, vars),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['env', app.id] });
      notifications.show({ color: 'green', message: 'Saved. Restart the app to apply.' });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  const submit = () => {
    try {
      setParseError('');
      save.mutate(parseDotenv(text));
    } catch (err) {
      setParseError((err as Error).message);
    }
  };

  return (
    <Card>
      <Stack>
        <div>
          <Text fw={600}>Environment variables</Text>
          <Text size="sm" c="dimmed">
            Write one <Code>KEY=value</Code> per line. Restart the app to apply changes.
          </Text>
          <Text size="sm" c="dimmed" mt={4}>
            Moonshed sets following env variables <Code>PORT</Code>, <Code>APP_NAME</Code>, <Code>RELEASE_ID</Code>
            {app.runtime === 'native' && (
              <>
                , <Code>APP_DIR</Code>, <Code>SHARED_DIR</Code>, <Code>CACHE_DIR</Code>
              </>
            )}.
          </Text>
        </div>
        {parseError && (
          <Alert color="red" variant="light">
            {parseError}
          </Alert>
        )}
        <Textarea
          value={text}
          onChange={(e) => setText(e.currentTarget.value)}
          autosize
          minRows={10}
          maxRows={30}
          styles={{ input: { fontFamily: 'var(--mantine-font-family-monospace)', fontSize: 13 } }}
          placeholder={'DATABASE_URL=postgres://...\nDEBUG=false'}
        />
        <Group justify="flex-end">
          <Button onClick={submit} loading={save.isPending}>
            Save
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}
