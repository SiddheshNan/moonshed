import { useState, type ReactNode } from 'react';
import { Button, Card, Group, NumberInput, PasswordInput, SegmentedControl, Select, SimpleGrid, Stack, Switch, Text, TextInput } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api, queryClient } from '../../api';
import type { App, Project, ServerInfo } from '../../types';

function useSave(app: App) {
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

function Section({
  title,
  description,
  children,
  onSave,
  saving,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onSave?: () => void;
  saving?: boolean;
}) {
  return (
    <Card padding="lg">
      <Text fw={600}>{title}</Text>
      {description && (
        <Text size="sm" c="dimmed">
          {description}
        </Text>
      )}
      <Stack mt="md">{children}</Stack>
      {onSave && (
        <Group justify="flex-end" mt="md">
          <Button size="sm" onClick={onSave} loading={saving}>
            Save
          </Button>
        </Group>
      )}
    </Card>
  );
}

export function AppSettings({ app }: { app: App }) {
  return (
    <Stack gap="lg" maw={760}>
      <General app={app} />
      {app.runtime !== 'docker' && <RunAs app={app} />}
      <Danger app={app} />
    </Stack>
  );
}

function General({ app }: { app: App }) {
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects') });
  const [form, setForm] = useState({
    project_id: app.project_id,
    runtime: app.runtime,
    keep_bundles: app.keep_bundles,
    log_retention_days: app.log_retention_days,
    auto_restart: app.auto_restart,
  });
  const save = useSave(app);
  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) => setForm((f) => ({ ...f, [key]: value }));

  return (
    <Section title="General" onSave={() => save.mutate(form)} saving={save.isPending}>
      <Select
        label="Project"
        data={(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))}
        value={form.project_id}
        onChange={(v) => v && set('project_id', v)}
        allowDeselect={false}
      />
      {!app.current_release_id && (
        <Stack gap={6}>
          <Text size="sm" fw={500}>
            Runtime
          </Text>
          <SegmentedControl
            w="fit-content"
            value={form.runtime}
            onChange={(v) => set('runtime', v as App['runtime'])}
            data={[
              { value: '', label: 'Detect from bundle' },
              { value: 'native', label: 'Native' },
              { value: 'docker', label: 'Docker' },
            ]}
          />
          <Text size="xs" c="dimmed">
            Detection picks native when app.yaml has a run script, otherwise Docker when there is a Dockerfile. Fixed after the first deploy.
          </Text>
        </Stack>
      )}
      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <NumberInput label="Versions to keep" min={1} value={form.keep_bundles} onChange={(v) => set('keep_bundles', Number(v) || 1)} />
        <NumberInput label="Keep logs for (days)" min={1} value={form.log_retention_days} onChange={(v) => set('log_retention_days', Number(v) || 1)} />
      </SimpleGrid>
      <Switch label="Restart automatically when the health check fails" checked={form.auto_restart} onChange={(e) => set('auto_restart', e.currentTarget.checked)} />
    </Section>
  );
}

function RunAs({ app }: { app: App }) {
  const server = useQuery({ queryKey: ['server'], queryFn: () => api.get<ServerInfo>('/server') });
  const windows = server.data?.stats.os === 'windows';
  const [user, setUser] = useState(app.run_as_user);
  const [password, setPassword] = useState('');
  const save = useSave(app);

  return (
    <Section
      title="Run as OS user"
      description="Run the app and its scripts as this user instead of the platform's service account. The user must already exist."
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
