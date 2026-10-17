import { useEffect, useState } from 'react';
import { Alert, Button, Group, Modal, SegmentedControl, Select, Stack, Text, TextInput } from '@mantine/core';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api, queryClient } from '../api';
import { emptyGitSource, gitSourceFields, GitSourceFields, gitSourceReady, type GitSourceValue } from '../components/GitSource';
import type { App, Project } from '../types';

type SourceChoice = 'upload' | 'github' | 'git';

const sourceHelp: Record<SourceChoice, string> = {
  upload: 'Upload a zip from the app page or from CI.',
  github: 'Pick a repository from your connected GitHub account.',
  git: 'Any git host: GitLab, Bitbucket, Gitea, or GitHub with a token.',
};

export function NewAppModal({ opened, onClose, projectId }: { opened: boolean; onClose: () => void; projectId?: string }) {
  const navigate = useNavigate();
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects'), enabled: opened });
  const [name, setName] = useState('');
  const [project, setProject] = useState<string | null>(null);
  const [source, setSource] = useState<SourceChoice>('upload');
  const [git, setGit] = useState<GitSourceValue>(emptyGitSource('github'));

  useEffect(() => {
    if (!opened) return;
    if (projectId) setProject(projectId);
    else if (!project && projects.data?.length) setProject(projects.data[0].id);
  }, [opened, projectId, projects.data, project]);

  const reset = () => {
    setName('');
    setSource('upload');
    setGit(emptyGitSource('github'));
  };

  const create = useMutation({
    mutationFn: () =>
      api.post<App>('/apps', {
        name,
        project_id: project ?? '',
        ...(source === 'upload' ? { source: 'upload' } : gitSourceFields(git)),
      }),
    onSuccess: (app) => {
      queryClient.invalidateQueries({ queryKey: ['apps'] });
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['project'] });
      onClose();
      reset();
      navigate(`/apps/${app.id}`);
    },
  });

  return (
    <Modal opened={opened} onClose={onClose} title="New app" size="md">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <Stack gap="md">
          {create.error && (
            <Alert color="red" variant="light">
              {create.error.message}
            </Alert>
          )}
          <TextInput
            label="Name"
            placeholder="my-app"
            description="Lowercase letters, digits and dashes"
            value={name}
            onChange={(e) => setName(e.currentTarget.value.toLowerCase())}
            required
            data-autofocus
          />
          {(projects.data?.length ?? 0) > 1 && (
            <Select
              label="Project"
              data={(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))}
              value={project}
              onChange={setProject}
              allowDeselect={false}
            />
          )}
          <Stack gap={6}>
            <Text size="sm" fw={500}>
              Deploy from
            </Text>
            <SegmentedControl
              fullWidth
              value={source}
              onChange={(v) => {
                const choice = v as SourceChoice;
                setSource(choice);
                if (choice !== 'upload') setGit(emptyGitSource(choice));
              }}
              data={[
                { value: 'upload', label: 'Zip upload' },
                { value: 'github', label: 'GitHub' },
                { value: 'git', label: 'Git URL' },
              ]}
            />
            <Text size="xs" c="dimmed">
              {sourceHelp[source]}
            </Text>
          </Stack>
          {source !== 'upload' && <GitSourceFields value={git} onChange={setGit} />}
          <Text size="xs" c="dimmed">
            Native or Docker is detected from the bundle: app.yaml runs natively, a Dockerfile runs in Docker. Set the port in app.yaml
            or later on the app page.
          </Text>
          <Group justify="flex-end" gap="sm">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={create.isPending} disabled={!name || (source !== 'upload' && !gitSourceReady(git))}>
              Create app
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
