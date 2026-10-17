import { useState } from 'react';
import { Button, Code, CopyButton, Group, Progress, SegmentedControl, Stack, Text } from '@mantine/core';
import { Dropzone } from '@mantine/dropzone';
import { notifications } from '@mantine/notifications';
import { IconFileZip, IconUpload, IconX } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api, queryClient, uploadBundle } from '../../api';
import { emptyGitSource, gitSourceFields, GitSourceFields, gitSourceFromApp, gitSourceReady, type GitSourceValue } from '../../components/GitSource';
import { appPath } from '../../paths';
import type { App, SettingsResponse } from '../../types';
import { Section, useSave } from './Settings';

type SourceChoice = 'upload' | 'github' | 'git';

// Source picks where new versions come from. Switching doesn't touch the running version;
// the next deploy uses the new source.
export function Source({ app }: { app: App }) {
  const current: SourceChoice = app.source === 'git' ? (app.github_installation_id ? 'github' : 'git') : 'upload';
  const [choice, setChoice] = useState<SourceChoice>(current);
  const [git, setGit] = useState<GitSourceValue>(() => (current === 'upload' ? emptyGitSource('github') : gitSourceFromApp(app)));
  const save = useSave(app);
  const hookURL = `${usePanelOrigin()}/api/hooks/apps/${app.id}`;

  const showWebhook = current === 'git' && choice === 'git' && app.autodeploy && !!app.webhook_secret;

  const pick = (v: SourceChoice) => {
    setChoice(v);
    if (v !== 'upload' && v !== git.provider) setGit(v === current ? gitSourceFromApp(app) : emptyGitSource(v));
  };
  const toFields = (c: SourceChoice, g: GitSourceValue) =>
    c === 'upload' ? { source: 'upload', git_url: '', git_branch: '', git_subdir: '', git_credential_id: '', github_installation_id: 0 } : gitSourceFields(g);
  const fields = toFields(choice, git);
  const changed = JSON.stringify(fields) !== JSON.stringify(toFields(current, current === 'upload' ? git : gitSourceFromApp(app)));

  return (
    <Section
      title="Source"
      onSave={changed ? () => save.mutate(fields) : undefined}
      saving={save.isPending}
      saveDisabled={choice !== 'upload' && !gitSourceReady(git)}
    >
      <SegmentedControl
        w="fit-content"
        value={choice}
        onChange={(v) => pick(v as SourceChoice)}
        data={[
          { value: 'upload', label: 'Zip upload' },
          { value: 'github', label: 'GitHub' },
          { value: 'git', label: 'Git URL' },
        ]}
      />
      {choice !== 'upload' && <GitSourceFields value={git} onChange={setGit} webhookShown={showWebhook} />}
      {choice === 'upload' &&
        (current === 'upload' ? (
          <UploadBundle app={app} />
        ) : (
          <Text size="sm" c="dimmed">
            Save to switch to zip uploads.
          </Text>
        ))}
      {showWebhook && (
        <Stack gap={6}>
          <div>
            <Text size="sm" fw={500}>
              Add this webhook to your repository
            </Text>
            <Text size="xs" c="dimmed">
              Pushes only deploy once it's added. Use push events; on GitHub, set the content type to application/json.
            </Text>
          </div>
          <CopyField label="URL" value={hookURL} />
          <CopyField label="Secret" value={app.webhook_secret ?? ''} />
        </Stack>
      )}
    </Section>
  );
}

// usePanelOrigin is the panel's public address when it has a domain, else the one in the browser.
// Git hosts and CI must reach it, so the public domain wins.
function usePanelOrigin(): string {
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const s = settings.data?.settings;
  return s?.panel_domain && s.proxy_enabled ? `${s.panel_tls === 'none' ? 'http' : 'https'}://${s.panel_domain}` : location.origin;
}

export function CopyField({ label, value }: { label?: string; value: string }) {
  return (
    <Group gap={6} wrap="nowrap">
      {label && (
        <Text size="xs" c="dimmed" w={48} style={{ flex: 'none' }}>
          {label}
        </Text>
      )}
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

// UploadBundle is the zip drop, plus the command to deploy from CI.
export function UploadBundle({ app }: { app: App }) {
  const navigate = useNavigate();
  const [progress, setProgress] = useState<number | null>(null);

  const upload = async (file: File) => {
    setProgress(0);
    try {
      await uploadBundle(app.id, file, setProgress);
      queryClient.invalidateQueries({ queryKey: ['deployments', app.id] });
      navigate(appPath(app.project_id, app.id, 'deployments'));
    } catch (err) {
      notifications.show({ color: 'red', title: 'Upload failed', message: (err as Error).message });
    } finally {
      setProgress(null);
    }
  };

  return (
    <Stack gap="lg">
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
      <CIDeploy app={app} />
    </Stack>
  );
}

// CIDeploy creates an API token on the spot and fills it into the deploy command.
function CIDeploy({ app }: { app: App }) {
  const origin = usePanelOrigin();
  const [token, setToken] = useState<string | null>(null);
  const create = useMutation({
    mutationFn: () => api.post<{ token: string }>('/tokens', { name: `ci-${app.name}` }),
    onSuccess: (res) => {
      setToken(res.token);
      queryClient.invalidateQueries({ queryKey: ['tokens'] });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const curl = `curl -fsS -H "Authorization: Bearer ${token ?? '$MOONSHED_TOKEN'}" -H "Content-Type: application/zip" --data-binary @app.zip ${origin}/api/apps/${app.id}/deploy`;

  return (
    <Stack gap={6}>
      <Group justify="space-between">
        <Text size="sm" fw={500}>
          Deploy from CI
        </Text>
        {!token && (
          <Button size="compact-sm" variant="default" loading={create.isPending} onClick={() => create.mutate()}>
            Create token
          </Button>
        )}
      </Group>
      {token && (
        <Text size="xs" c="orange">
          Copy it now. The token won't be shown again; manage tokens in Settings › API tokens.
        </Text>
      )}
      <CopyField value={curl} />
    </Stack>
  );
}
