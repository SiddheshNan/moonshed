import { useState, type ReactNode } from 'react';
import { Button, Card, Group, PasswordInput, SimpleGrid, Stack, Text, TextInput } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api, queryClient } from '../../api';
import type { App, ServerInfo } from '../../types';

export function useSave(app: App) {
  return useMutation({
    mutationFn: (fields: Record<string, unknown>) => api.put<App>(`/apps/${app.id}`, fields),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['app', app.id] });
      queryClient.invalidateQueries({ queryKey: ['apps'] });
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      notifications.show({ color: 'green', message: 'Saved' });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
}

export function Section({
  title,
  description,
  children,
  onSave,
  saving,
  saveDisabled,
  saveHint,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onSave?: () => void;
  saving?: boolean;
  saveDisabled?: boolean;
  // saveHint is a short note shown beside Save, e.g. when the change takes effect.
  saveHint?: string;
}) {
  return (
    <Card padding={0}>
      <Group className="panel-header" justify="space-between" wrap="nowrap">
        <div>
          <Text fw={600}>{title}</Text>
          {description && (
            <Text size="sm" c="dimmed">
              {description}
            </Text>
          )}
        </div>
        {onSave && (
          <Group gap="sm" wrap="nowrap">
            {saveHint && (
              <Text size="xs" c="dimmed">
                {saveHint}
              </Text>
            )}
            <Button size="sm" onClick={onSave} loading={saving} disabled={saveDisabled}>
              Save
            </Button>
          </Group>
        )}
      </Group>
      <Stack p="lg">{children}</Stack>
    </Card>
  );
}

export function AppSettings({ app }: { app: App }) {
  return (
    <Stack gap="lg" maw={760}>
      {app.runtime !== 'docker' && <RunAs app={app} />}
      <Danger app={app} />
    </Stack>
  );
}

function RunAs({ app }: { app: App }) {
  const server = useQuery({ queryKey: ['server'], queryFn: () => api.get<ServerInfo>('/server') });
  const windows = server.data?.stats?.os === 'windows';
  const [user, setUser] = useState(app.run_as_user);
  const [password, setPassword] = useState('');
  const save = useSave(app);

  return (
    <Section
      title="Run as OS user"
      description="Runs the app and its scripts as an existing OS user instead of Moonshed's service account."
      onSave={() => save.mutate({ run_as_user: user, ...(password ? { run_as_password: password } : {}) }, { onSuccess: () => setPassword('') })}
      saving={save.isPending}
    >
      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <TextInput
          label="Username"
          placeholder={windows ? 'DOMAIN\\user' : 'Empty = service account'}
          value={user}
          onChange={(e) => setUser(e.currentTarget.value)}
        />
        {windows && (
          <PasswordInput
            label="Password"
            description="Windows needs it to start processes as the user"
            placeholder={app.has_run_as_password ? '•••••••• (unchanged)' : ''}
            value={password}
            onChange={(e) => setPassword(e.currentTarget.value)}
          />
        )}
      </SimpleGrid>
    </Section>
  );
}

function Danger({ app }: { app: App }) {
  const navigate = useNavigate();
  const remove = useMutation({
    mutationFn: () => api.delete(`/apps/${app.id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['apps'] });
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      navigate(`/projects/${app.project_id}`);
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  return (
    <Card padding="lg" style={{ borderColor: 'var(--mantine-color-red-9)' }}>
      <Group justify="space-between" wrap="nowrap">
        <div>
          <Text fw={600}>Delete app</Text>
          <Text size="sm" c="dimmed">
            Stops the app and deletes its versions, data, logs and backups.
          </Text>
        </div>
        <Button
          color="red"
          variant="light"
          loading={remove.isPending}
          onClick={() =>
            modals.openConfirmModal({
              title: `Delete ${app.name}?`,
              children: <Text size="sm">This can't be undone.</Text>,
              labels: { confirm: 'Delete', cancel: 'Cancel' },
              confirmProps: { color: 'red' },
              onConfirm: () => remove.mutate(),
            })
          }
        >
          Delete
        </Button>
      </Group>
    </Card>
  );
}
