import { useState, type ReactNode } from 'react';
import { ActionIcon, Alert, Badge, Button, Center, Loader, Modal, Stack, Tabs, Text, Tooltip } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { IconPencil, IconPlayerPlay, IconPlayerStop, IconRefresh, IconRocket } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Navigate, useNavigate, useParams } from 'react-router';

import { api, queryClient } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import { Commit } from '../../components/Commit';
import { AppStatusBadge } from '../../components/StatusBadge';
import { TimeAgo } from '../../components/TimeAgo';
import { runtimeLabel, triggerLabel } from '../../format';
import { appPath } from '../../paths';
import type { App, Project } from '../../types';
import { Backups } from './Backups';
import { Deployments } from './Deployments';
import { Domains } from './Domains';
import { Environment } from './Environment';
import { Logs } from './Logs';
import { MoveModal } from './MoveModal';
import { Monitoring } from './Monitoring';
import { Overview } from './Overview';
import { UploadBundle } from './Source';
import { AppSettings } from './Settings';

const tabs = [
  { value: 'overview', label: 'Overview' },
  { value: 'deployments', label: 'Deployments' },
  { value: 'logs', label: 'Logs' },
  { value: 'monitoring', label: 'Monitoring' },
  { value: 'environment', label: 'Environment' },
  { value: 'domains', label: 'Domains' },
  { value: 'backups', label: 'Backups' },
  { value: 'advanced', label: 'Advanced' },
];

// deployedSummary says what is running: when it was deployed and from which commit or upload.
function deployedSummary(a: App): ReactNode {
  const d = a.deployed;
  if (!d) return a.current_release_id ? 'Deployed' : 'Not deployed yet';
  const when = <TimeAgo iso={d.finished_at ?? d.created_at} prefix="Deployed " />;
  if (!d.git_commit) return (
    <>
      {when} · {triggerLabel(d.trigger)}
    </>
  );
  return (
    <>
      {when} · {a.git_branch || 'main'} @ <Commit gitUrl={a.git_url} sha={d.git_commit} message={d.git_message} />
      {d.git_message && (
        <>
          {' · '}
          <Tooltip label={d.git_message} multiline maw={480} style={{ whiteSpace: 'pre-wrap' }} openDelay={300}>
            <Text span inherit truncate maw="min(640px, 60vw)" display="inline-block" style={{ verticalAlign: 'bottom' }}>
              {d.git_message.split('\n')[0]}
            </Text>
          </Tooltip>
        </>
      )}
    </>
  );
}

export function AppPage() {
  const params = useParams();
  const navigate = useNavigate();
  const id = params.id ?? '';
  const projectId = params.projectId ?? '';
  const tab = params['*'] || 'overview';
  const [uploading, setUploading] = useState(false);
  const [moving, setMoving] = useState(false);
  const app = useQuery({ queryKey: ['app', id], queryFn: () => api.get<App>(`/apps/${id}`), refetchInterval: 3000 });
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects') });

  const action = useMutation({
    mutationFn: (name: 'start' | 'stop' | 'restart' | 'deploy') => api.post(`/apps/${id}/${name}`),
    onSuccess: (_, name) => {
      queryClient.invalidateQueries({ queryKey: ['app', id] });
      queryClient.invalidateQueries({ queryKey: ['apps'] });
      queryClient.invalidateQueries({ queryKey: ['deployments', id] });
      if (name === 'deploy') navigate(appPath(projectId, id, 'deployments'));
    },
    onError: (err) => notifications.show({ color: 'red', title: 'Action failed', message: err.message }),
  });

  if (app.isPending) {
    return (
      <Center h={300}>
        <Loader />
      </Center>
    );
  }
  if (app.error || !app.data) {
    return <Alert color="red">{app.error?.message ?? 'App not found'}</Alert>;
  }

  const a = app.data;
  if (a.project_id !== projectId) {
    // The app was moved to another project; follow it.
    return <Navigate to={appPath(a.project_id, a.id, tab === 'overview' ? '' : tab)} replace />;
  }
  const project = projects.data?.find((p) => p.id === a.project_id);
  const running = a.status.state === 'running' || a.status.state === 'crashed';

  return (
    <Stack gap="lg">
      <PageHeader
        crumbs={[
          { label: 'Projects', to: '/projects' },
          { label: project?.name ?? '…', to: `/projects/${a.project_id}` },
          { label: a.name },
        ]}
        title={a.name}
        badges={
          <>
            <AppStatusBadge app={a} />
            {a.runtime && (
              <Badge variant="default" fw={500}>
                {runtimeLabel(a.runtime)}
              </Badge>
            )}
            <Tooltip label="Move to another project">
              <ActionIcon onClick={() => setMoving(true)}>
                <IconPencil size={16} />
              </ActionIcon>
            </Tooltip>
          </>
        }
        description={deployedSummary(a)}
        actions={
          <>
            <Button
              leftSection={<IconRocket size={16} />}
              onClick={() => (a.source === 'git' ? action.mutate('deploy') : setUploading(true))}
              loading={action.isPending && action.variables === 'deploy'}
            >
              Deploy
            </Button>
            {running ? (
              <>
                <Button
                  variant="default"
                  leftSection={<IconRefresh size={16} />}
                  onClick={() => action.mutate('restart')}
                  loading={action.isPending && action.variables === 'restart'}
                >
                  Restart
                </Button>
                <Button
                  variant="default"
                  color="red"
                  leftSection={<IconPlayerStop size={16} />}
                  onClick={() => action.mutate('stop')}
                  loading={action.isPending && action.variables === 'stop'}
                >
                  Stop
                </Button>
              </>
            ) : a.current_release_id && (
              <Button
                variant="default"
                leftSection={<IconPlayerPlay size={16} />}
                onClick={() => action.mutate('start')}
                loading={action.isPending && action.variables === 'start'}
              >
                Start
              </Button>
            )}
          </>
        }
      />

      <Tabs value={tab} onChange={(v) => navigate(appPath(projectId, id, v === 'overview' ? '' : (v ?? '')))} keepMounted={false}>
        <Tabs.List mb="lg">
          {tabs.map((t) => (
            <Tabs.Tab key={t.value} value={t.value}>
              {t.label}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        <Tabs.Panel value="overview">
          <Overview app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="deployments">
          <Deployments app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="logs">
          <Logs app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="monitoring">
          <Monitoring app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="environment">
          <Environment app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="domains">
          <Domains app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="backups">
          <Backups app={a} />
        </Tabs.Panel>
        <Tabs.Panel value="advanced">
          <AppSettings app={a} />
        </Tabs.Panel>
      </Tabs>
      <Modal opened={uploading} onClose={() => setUploading(false)} title={`Deploy ${a.name}`} size="lg">
        <UploadBundle app={a} />
      </Modal>
      <MoveModal app={a} opened={moving} onClose={() => setMoving(false)} />
    </Stack>
  );
}
