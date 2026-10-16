import { Anchor, Group, Select, Stack, Switch, Text, TextInput } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api } from '../api';
import type { App, GitOverview, Repo } from '../types';

export interface GitSourceValue {
  provider: 'github' | 'git';
  installation: number;
  repo: string;
  gitURL: string;
  branch: string;
  subdir: string;
  credential: string;
  autodeploy: boolean;
}

export function emptyGitSource(provider: GitSourceValue['provider']): GitSourceValue {
  return { provider, installation: 0, repo: '', gitURL: '', branch: 'main', subdir: '', credential: '', autodeploy: true };
}

export function gitSourceFromApp(app: App): GitSourceValue {
  const repo = app.git_url.replace(/^https?:\/\/[^/]+\//, '').replace(/\.git$/, '');
  return {
    provider: app.github_installation_id ? 'github' : 'git',
    installation: app.github_installation_id,
    repo: app.github_installation_id ? repo : '',
    gitURL: app.git_url,
    branch: app.git_branch || 'main',
    subdir: app.git_subdir,
    credential: app.git_credential_id,
    autodeploy: app.autodeploy,
  };
}

// gitSourceFields turns the form value into the app API's fields.
export function gitSourceFields(v: GitSourceValue) {
  const github = v.provider === 'github';
  return {
    source: 'git' as const,
    git_url: v.gitURL,
    git_branch: v.branch,
    git_subdir: v.subdir,
    git_credential_id: github ? '' : v.credential,
    github_installation_id: github ? v.installation : 0,
    autodeploy: v.autodeploy,
  };
}

export function gitSourceReady(v: GitSourceValue): boolean {
  return v.provider === 'github' ? !!(v.installation && v.repo && v.branch) : !!(v.gitURL && v.branch);
}

// GitSourceFields edits a repository source. The provider (GitHub App or any git URL) is chosen by the caller.
export function GitSourceFields({ value, onChange }: { value: GitSourceValue; onChange: (v: GitSourceValue) => void }) {
  const set = (patch: Partial<GitSourceValue>) => onChange({ ...value, ...patch });
  const git = useQuery({ queryKey: ['git'], queryFn: () => api.get<GitOverview>('/git') });
  const installs = git.data?.github_installations ?? [];
  const installation = value.installation || (installs.length === 1 ? installs[0].id : 0);
  const repos = useQuery({
    queryKey: ['git-repos', installation],
    queryFn: () => api.get<Repo[]>(`/git/installations/${installation}/repos`),
    enabled: value.provider === 'github' && installation > 0,
  });
  const branches = useQuery({
    queryKey: ['git-branches', installation, value.repo],
    queryFn: () => api.get<string[]>(`/git/installations/${installation}/branches?repo=${encodeURIComponent(value.repo)}`),
    enabled: value.provider === 'github' && installation > 0 && !!value.repo,
  });

  const autodeploy = (
    <Switch
      label="Deploy automatically on every push"
      checked={value.autodeploy}
      onChange={(e) => set({ autodeploy: e.currentTarget.checked })}
    />
  );

  if (value.provider === 'github') {
    if (git.data && installs.length === 0) {
      return (
        <Text size="sm" c="dimmed">
          GitHub isn't connected yet.{' '}
          <Anchor component={Link} to="/settings/git" size="sm">
            Connect GitHub
          </Anchor>{' '}
          first, or use a Git URL.
        </Text>
      );
    }
    return (
      <Stack gap="sm">
        {installs.length > 1 && (
          <Select
            label="GitHub account"
            data={installs.map((i) => ({ value: String(i.id), label: i.account }))}
            value={installation ? String(installation) : null}
            onChange={(v) => set({ installation: Number(v) || 0, repo: '', gitURL: '' })}
            allowDeselect={false}
          />
        )}
        <Select
          label="Repository"
          placeholder={repos.isFetching ? 'Loading…' : 'Pick a repository'}
          searchable
          nothingFoundMessage={repos.error ? repos.error.message : 'No repositories'}
          disabled={!installation}
          data={(repos.data ?? []).map((r) => ({ value: r.full_name, label: r.full_name }))}
          value={value.repo || null}
          onChange={(name) => {
            const repo = repos.data?.find((r) => r.full_name === name);
            if (repo) set({ installation, repo: repo.full_name, gitURL: repo.clone_url, branch: repo.default_branch });
          }}
        />
        <Group grow align="flex-start">
          <Select
            label="Branch"
            searchable
            disabled={!value.repo}
            data={branches.data ?? (value.branch ? [value.branch] : [])}
            value={value.branch || null}
            onChange={(v) => set({ branch: v ?? '' })}
            allowDeselect={false}
          />
          <TextInput label="Folder" placeholder="repository root" value={value.subdir} onChange={(e) => set({ subdir: e.currentTarget.value })} />
        </Group>
        {autodeploy}
      </Stack>
    );
  }

  return (
    <Stack gap="sm">
      <TextInput
        label="Repository URL"
        placeholder="https://gitlab.com/you/app.git"
        value={value.gitURL}
        onChange={(e) => set({ gitURL: e.currentTarget.value })}
      />
      <Select
        label="Access token"
        description={
          <>
            For private repositories. Add tokens in{' '}
            <Anchor component={Link} to="/settings/git" size="xs">
              Settings › Git
            </Anchor>
          </>
        }
        data={[
          { value: '', label: 'None (public repository)' },
          ...(git.data?.credentials ?? []).map((c) => ({ value: c.id, label: `${c.name} · ${c.host}` })),
        ]}
        value={value.credential}
        onChange={(v) => set({ credential: v ?? '' })}
        allowDeselect={false}
      />
      <Group grow align="flex-start">
        <TextInput label="Branch" value={value.branch} onChange={(e) => set({ branch: e.currentTarget.value })} />
        <TextInput label="Folder" placeholder="repository root" value={value.subdir} onChange={(e) => set({ subdir: e.currentTarget.value })} />
      </Group>
      {autodeploy}
    </Stack>
  );
}
