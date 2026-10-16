import { useState, type ReactNode } from 'react';
import {
  ActionIcon,
  Alert,
  Anchor,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Modal,
  NumberInput,
  Progress,
  Stack,
  Switch,
  Text,
  Tooltip,
} from '@mantine/core';
import { Dropzone } from '@mantine/dropzone';
import { notifications } from '@mantine/notifications';
import { IconBrandGithub, IconFileZip, IconGitBranch, IconPencil, IconRocket, IconUpload, IconX } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Link, useNavigate } from 'react-router';

import { api, queryClient, uploadBundle } from '../../api';
import { gitSourceFields, GitSourceFields, gitSourceFromApp, gitSourceReady, type GitSourceValue } from '../../components/GitSource';
import { formatBytes, formatPercent, summarizeError, timeAgo } from '../../format';
import type { App, Deployment } from '../../types';

function Fact({ label, children, action }: { label: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="fact">
      <Group justify="space-between" gap={4} wrap="nowrap">
        <div className="fact-label">{label}</div>
        {action}
      </Group>
      <Text size="sm" component="div" truncate>
        {children}
      </Text>
    </div>
  );
}

function Dimmed({ children }: { children: ReactNode }) {
  return (
    <Text span c="dimmed" size="sm">
      {children}
    </Text>
  );
}

function useUpdateApp(app: App) {
  return useMutation({
    mutationFn: (fields: Record<string, unknown>) => api.put<App>(`/apps/${app.id}`, fields),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['app', app.id] });
      queryClient.invalidateQueries({ queryKey: ['apps'] });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
}

export function Overview({ app }: { app: App }) {
  const deployments = useQuery({
    queryKey: ['deployments', app.id],
    queryFn: () => api.get<Deployment[]>(`/apps/${app.id}/deployments`),
    refetchInterval: 5000,
  });
  const latest = deployments.data?.[0];
  const deployed = !!app.current_release_id;

  return (
    <Stack gap="lg">
      {app.source === 'git' && app.pushed_commit && (
        <Alert color="indigo" variant="light" title="New commit waiting to deploy">
          <Code>{app.pushed_commit.slice(0, 7)}</Code> {app.pushed_message}. Autodeploy is off, so press Deploy to ship it.
        </Alert>
      )}
      {latest?.status === 'failed' && (
        <Alert color="red" variant="light" title="The last deployment failed">
          <Text size="sm">
            {summarizeError(latest.error)}{' '}
            <Anchor component={Link} to={`/apps/${app.id}/deployments`} size="sm">
              View log
            </Anchor>
          </Text>
        </Alert>
      )}
      {deployed && <Facts app={app} />}
      {app.source === 'git' ? <GitSource app={app} /> : <UploadBundle app={app} />}
    </Stack>
  );
}

function Facts({ app }: { app: App }) {
  const [editingPort, setEditingPort] = useState(false);
  const running = app.status.state === 'running';
  const measured = running && app.stats.processes > 0;
  return (
    <Card padding={0}>
      <div className="facts">
        <Fact label="Status">
          {running && app.status.since ? `Up ${timeAgo(app.status.since).replace(' ago', '')}` : app.status.state === 'crashed' ? 'Crashed' : 'Stopped'}
          {app.status.restarts > 0 && <Dimmed> · {app.status.restarts} restarts</Dimmed>}
        </Fact>
        <Fact
          label="Port"
          action={
            <Tooltip label="Change port">
              <ActionIcon size="xs" onClick={() => setEditingPort(true)}>
                <IconPencil size={12} />
              </ActionIcon>
            </Tooltip>
          }
        >
          {app.port || <Text span c="orange" size="sm">Not set</Text>}
        </Fact>
        <Fact label="Runtime">{app.runtime === 'docker' ? 'Docker' : 'Native process'}</Fact>
        <Fact label="Health check">{app.health_check || <Dimmed>Process is running</Dimmed>}</Fact>
        <Fact label="CPU">{measured ? formatPercent(app.stats.cpu, 2) : <Dimmed>Not running</Dimmed>}</Fact>
        <Fact label="Memory">{measured ? formatBytes(app.stats.memory) : <Dimmed>Not running</Dimmed>}</Fact>
        <Fact label="Disk">{formatBytes(app.stats.disk)}</Fact>
        <Fact label="Domains">
          {app.domains.length === 0 ? (
            <Anchor component={Link} to={`/apps/${app.id}/domains`} size="sm">
              Add a domain
            </Anchor>
          ) : (
            app.domains.map((d) => (
              <Anchor key={d.id} href={`${d.tls === 'none' ? 'http' : 'https'}://${d.host}${d.path}`} target="_blank" size="sm" display="block" truncate>
                {d.host}
                {d.path === '/' ? '' : d.path}
              </Anchor>
            ))
          )}
        </Fact>
      </div>
      <PortModal app={app} opened={editingPort} onClose={() => setEditingPort(false)} />
    </Card>
  );
}

function PortModal({ app, opened, onClose }: { app: App; opened: boolean; onClose: () => void }) {
  const [port, setPort] = useState<number | string>(app.port || '');
  const update = useUpdateApp(app);
  return (
    <Modal opened={opened} onClose={onClose} title="Port" size="sm">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          update.mutate({ port: Number(port) || 0 }, { onSuccess: onClose });
        }}
      >
        <Stack>
          <NumberInput
            label="Port the app listens on"
            description="Passed to the app as $PORT. Used for domains and health checks. Takes effect on the next start."
            min={1}
            max={65535}
            value={port}
            onChange={setPort}
            data-autofocus
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={update.isPending}>
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}

function GitSource({ app }: { app: App }) {
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  const update = useUpdateApp(app);
  const deploy = useMutation({
    mutationFn: () => api.post(`/apps/${app.id}/deploy`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployments', app.id] });
      navigate(`/apps/${app.id}/deployments`);
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const repo = app.git_url.replace(/^https?:\/\//, '').replace(/\.git$/, '');
  const hookURL = `${location.origin}/api/hooks/apps/${app.id}`;

  return (
    <Card padding="lg">
      <Group justify="space-between" align="flex-start" wrap="nowrap">
        <Group gap="sm" wrap="nowrap" style={{ minWidth: 0 }}>
          {app.github_installation_id ? <IconBrandGithub size={20} /> : <IconGitBranch size={20} />}
          <div style={{ minWidth: 0 }}>
            <Anchor href={app.git_url.replace(/\.git$/, '')} target="_blank" fw={500} size="sm" display="block" truncate>
              {repo}
            </Anchor>
            <Text size="xs" c="dimmed">
              {app.git_branch || 'main'}
              {app.git_subdir ? ` · /${app.git_subdir}` : ''}
              {app.deployed?.git_commit ? ` · deployed ${app.deployed.git_commit.slice(0, 7)}` : ''}
            </Text>
          </div>
        </Group>
        <Group gap="xs" wrap="nowrap">
          <Button size="xs" variant="default" leftSection={<IconPencil size={14} />} onClick={() => setEditing(true)}>
            Edit
          </Button>
          {!app.current_release_id && (
            <Button size="xs" leftSection={<IconRocket size={14} />} onClick={() => deploy.mutate()} loading={deploy.isPending}>
              Deploy now
            </Button>
          )}
        </Group>
      </Group>
      <Switch
        mt="md"
        label="Deploy automatically on every push"
        checked={app.autodeploy}
        disabled={update.isPending}
        onChange={(e) => update.mutate({ autodeploy: e.currentTarget.checked })}
      />
      {!app.github_installation_id && app.autodeploy && app.webhook_secret && (
        <Stack gap={4} mt="md">
          <Text size="xs" c="dimmed">
            For pushes to reach this app, add a webhook in the repository (GitHub: content type application/json; GitLab: push events)
            with this URL and secret:
          </Text>
          <CopyField value={hookURL} />
          <CopyField value={app.webhook_secret} />
        </Stack>
      )}
      <SourceModal app={app} opened={editing} onClose={() => setEditing(false)} />
    </Card>
  );
}

function CopyField({ value }: { value: string }) {
  return (
    <Group gap={6} wrap="nowrap">
      <Code style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{value}</Code>
      <CopyButton value={value}>
        {({ copied, copy }) => (
          <Button size="compact-xs" variant="subtle" color="gray" onClick={copy}>
            {copied ? 'Copied' : 'Copy'}
          </Button>
        )}
      </CopyButton>
    </Group>
  );
}

function SourceModal({ app, opened, onClose }: { app: App; opened: boolean; onClose: () => void }) {
  const [value, setValue] = useState<GitSourceValue>(() => gitSourceFromApp(app));
  const update = useUpdateApp(app);
  return (
    <Modal opened={opened} onClose={onClose} title="Repository" size="md">
      <Stack>
        <GitSourceFields value={value} onChange={setValue} />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!gitSourceReady(value)} loading={update.isPending} onClick={() => update.mutate(gitSourceFields(value), { onSuccess: onClose })}>
            Save
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}

function UploadBundle({ app }: { app: App }) {
  const navigate = useNavigate();
  const [progress, setProgress] = useState<number | null>(null);
  const curl = `curl -fsS -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/zip" --data-binary @app.zip ${location.origin}/api/apps/${app.id}/deploy`;

  const upload = async (file: File) => {
    setProgress(0);
    try {
      await uploadBundle(app.id, file, setProgress);
      queryClient.invalidateQueries({ queryKey: ['deployments', app.id] });
      navigate(`/apps/${app.id}/deployments`);
    } catch (err) {
      notifications.show({ color: 'red', title: 'Upload failed', message: (err as Error).message });
    } finally {
      setProgress(null);
    }
  };

  return (
    <Card padding="lg">
      <Text size="sm" fw={500}>
        {app.current_release_id ? 'Deploy a new version' : `Deploy ${app.name}`}
      </Text>
      <Text size="xs" c="dimmed" mb="sm">
        A zip with app.yaml (runs natively) or a Dockerfile (runs in Docker) at the root.
      </Text>
      {progress !== null ? (
        <Stack gap="xs" py="lg">
          <Text size="sm">Uploading… {progress}%</Text>
          <Progress value={progress} animated />
        </Stack>
      ) : (
        <Dropzone onDrop={(files) => upload(files[0])} accept={['application/zip', 'application/x-zip-compressed']} multiple={false} maxSize={4 * 1024 ** 3}>
          <Group justify="center" gap="md" mih={88} style={{ pointerEvents: 'none' }}>
            <Dropzone.Accept>
              <IconUpload size={28} stroke={1.5} />
            </Dropzone.Accept>
            <Dropzone.Reject>
              <IconX size={28} stroke={1.5} />
            </Dropzone.Reject>
            <Dropzone.Idle>
              <IconFileZip size={28} stroke={1.5} color="var(--mantine-color-dimmed)" />
            </Dropzone.Idle>
            <Text size="sm">
              Drop a .zip here or{' '}
              <Text span c="indigo" inherit>
                browse
              </Text>
            </Text>
          </Group>
        </Dropzone>
      )}
      <Text size="xs" c="dimmed" mt="md" mb={6}>
        From CI, with an API token from Settings:
      </Text>
      <CopyField value={curl} />
    </Card>
  );
}
