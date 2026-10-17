import { useState, type ReactNode } from 'react';
import {
  Alert,
  Anchor,
  Button,
  Card,
  Group,
  Modal,
  NumberInput,
  Stack,
  Text,
} from '@mantine/core';
import { notifications } from '@mantine/notifications';

import { useMutation, useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api, queryClient } from '../../api';
import { Commit } from '../../components/Commit';
import { appLook, Dot } from '../../components/StatusBadge';
import { formatBytes, formatPercent, summarizeError, timeAgo } from '../../format';
import { appPath } from '../../paths';
import type { App, Deployment } from '../../types';
import { Source } from './Source';

function Fact({ label, children, action }: { label: string; children: ReactNode; action?: ReactNode }) {
  return (
    <div className="fact">
      <Group justify="space-between" gap={4} wrap="nowrap">
        <div className="fact-label">{label}</div>
        {action}
      </Group>
      <Text size="sm" component="div" className="fact-value" truncate>
        {children}
      </Text>
    </div>
  );
}

function Dimmed({ children }: { children: ReactNode }) {
  return (
    <Text span c="dimmed" size="sm" fw={400}>
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
  const [editingPort, setEditingPort] = useState(false);
  const portProblem = latest?.status === 'failed' && /\bport\b/i.test(latest.error);

  return (
    <Stack gap="lg">
      {app.source === 'git' && app.pushed_commit && (
        <Alert color="indigo" variant="light" title="New commit waiting to deploy">
          <Commit gitUrl={app.git_url} sha={app.pushed_commit} /> {app.pushed_message}. Autodeploy is off.
        </Alert>
      )}
      {latest?.status === 'failed' && (
        <Alert color="red" variant="light" title="The last deployment failed">
          <Group justify="space-between" wrap="nowrap">
            <Text size="sm">
              {summarizeError(latest.error)}{' '}
              <Anchor component={Link} to={appPath(app.project_id, app.id, 'deployments')} size="sm">
                View log
              </Anchor>
            </Text>
            {portProblem && (
              <Button size="compact-sm" variant="default" onClick={() => setEditingPort(true)}>
                Change port
              </Button>
            )}
          </Group>
        </Alert>
      )}
      <Facts app={app} deployed={deployed} onEditPort={() => setEditingPort(true)} />
      <PortModal app={app} opened={editingPort} onClose={() => setEditingPort(false)} />
      <Source key={`${app.source}-${app.git_url}-${app.github_installation_id}`} app={app} />
    </Stack>
  );
}

function status(app: App): ReactNode {
  const look = appLook(app);
  const label = app.status.state === 'running' && app.status.since ? `Up ${timeAgo(app.status.since).replace(' ago', '')}` : look.label;
  return (
    <Group gap={8} wrap="nowrap">
      <Dot color={look.color} live={look.live} />
      {look.color === 'gray' ? <Dimmed>{label}</Dimmed> : <span>{label}</span>}
    </Group>
  );
}

function Facts({ app, deployed, onEditPort }: { app: App; deployed: boolean; onEditPort: () => void }) {
  const running = app.status.state === 'running';
  const measured = running && app.stats.processes > 0;
  return (
    <Card padding={0}>
      <div className="facts">
        <Fact label="Status">
          {status(app)}
          {app.status.restarts > 0 && <Dimmed> · {app.status.restarts} restarts</Dimmed>}
        </Fact>
        <Fact
          label="Port"
          action={
            <Anchor component="button" type="button" size="xs" onClick={onEditPort}>
              Edit
            </Anchor>
          }
        >
          {app.port || <Text span c="orange" size="sm">Not set</Text>}
        </Fact>
        <Fact label="Runtime">
          {app.runtime === 'docker' ? 'Docker' : app.runtime === 'native' ? 'Native process' : <Dimmed>Detected on first deploy</Dimmed>}
        </Fact>
        <Fact label="Health check">
          {!deployed ? <Dimmed>—</Dimmed> : app.health_check || <Dimmed>Process only</Dimmed>}
        </Fact>
        <Fact label="CPU">{measured ? formatPercent(app.stats.cpu, 2) : <Dimmed>—</Dimmed>}</Fact>
        <Fact label="Memory">{measured ? formatBytes(app.stats.memory) : <Dimmed>—</Dimmed>}</Fact>
        <Fact label="Disk">{deployed ? formatBytes(app.stats.disk) : <Dimmed>—</Dimmed>}</Fact>
        <Fact label="Domains">
          {app.domains.length === 0 ? (
            <Anchor component={Link} to={appPath(app.project_id, app.id, 'domains')} size="sm">
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
    </Card>
  );
}

export function PortModal({ app, opened, onClose }: { app: App; opened: boolean; onClose: () => void }) {
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
            description="Passed as $PORT. Applies on the next start."
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
