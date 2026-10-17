import { useState } from 'react';
import { Button, Code, CopyButton, Group, Progress, SegmentedControl, Stack, Text } from '@mantine/core';
import { Dropzone } from '@mantine/dropzone';
import { notifications } from '@mantine/notifications';
import { IconFileZip, IconUpload, IconX } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api, queryClient, uploadBundle } from '../../api';
import { emptyGitSource, gitSourceFields, GitSourceFields, gitSourceFromApp, gitSourceReady, type GitSourceValue } from '../../components/GitSource';
import type { App, SettingsResponse } from '../../types';
import { Section, useSave } from './Settings';

type SourceChoice = 'upload' | 'github' | 'git';

const sourceHelp: Record<SourceChoice, string> = {
  upload: 'Each version is a zip you upload, here or from CI.',
  github: 'Clones through the connected GitHub App, so private repositories need no token. Pushes arrive by webhook.',
  git: 'Any http(s) git URL: GitLab, Gitea, Bitbucket, self-hosted. Private repositories need an access token.',
};

// Source picks where new versions come from. Switching doesn't touch the running version;
// the next deploy uses the new source.
export function Source({ app }: { app: App }) {
  const current: SourceChoice = app.source === 'git' ? (app.github_installation_id ? 'github' : 'git') : 'upload';
  const [choice, setChoice] = useState<SourceChoice>(current);
  const [git, setGit] = useState<GitSourceValue>(() => (current === 'upload' ? emptyGitSource('github') : gitSourceFromApp(app)));
  const save = useSave(app);
  // Git hosts must reach the webhook, so prefer the panel's public domain over the address in the browser.
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const s = settings.data?.settings;
  const origin = s?.panel_domain && s.proxy_enabled ? `${s.panel_tls === 'none' ? 'http' : 'https'}://${s.panel_domain}` : location.origin;
  const hookURL = `${origin}/api/hooks/apps/${app.id}`;

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
      description={
        app.deployed?.git_commit
          ? `Deployed ${app.deployed.git_commit.slice(0, 7)}. Switching doesn't change what is running now; the next deploy uses the new source.`
          : "Where new versions come from. Switching doesn't change what is running now; the next deploy uses the new source."
      }
      onSave={changed ? () => save.mutate(fields) : undefined}
      saving={save.isPending}
      saveDisabled={choice !== 'upload' && !gitSourceReady(git)}
    >
      <Stack gap={6}>
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
        <Text size="xs" c="dimmed">
          {sourceHelp[choice]}
        </Text>
      </Stack>
      {choice !== 'upload' && <GitSourceFields value={git} onChange={setGit} />}
      {choice === 'upload' &&
        (current === 'upload' ? (
          <UploadBundle app={app} />
        ) : (
          <Text size="sm" c="dimmed">
            Save to switch this app to zip uploads, then drop a zip here to deploy it.
          </Text>
        ))}
      {current === 'git' && choice === 'git' && app.autodeploy && app.webhook_secret && (
        <Stack gap={4}>
          <Text size="xs" c="dimmed">
            For pushes to reach this app, add a webhook in the repository (GitHub: content type application/json; GitLab: push events) with this URL
            and secret:
          </Text>
          <CopyField value={hookURL} />
          <CopyField value={app.webhook_secret} />
        </Stack>
      )}
    </Section>
  );
}

export function CopyField({ value }: { value: string }) {
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

// UploadBundle is the zip drop: upload a bundle and follow its deployment.
export function UploadBundle({ app }: { app: App }) {
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

  const body = (
    <>
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
    </>
  );
  return body;
}
