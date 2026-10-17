import { useState } from 'react';
import {
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Modal,
  PasswordInput,
  Stack,
  Table,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconBrandGithub, IconPencil, IconPlus, IconRefresh, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useSearchParams } from 'react-router';

import { api, queryClient } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import { timeAgo } from '../../format';
import type { GitCredential, GitOverview } from '../../types';

export function GitSettingsPage() {
  const [params, setParams] = useSearchParams();
  const error = params.get('error');
  const git = useQuery({ queryKey: ['git'], queryFn: () => api.get<GitOverview>('/git') });

  return (
    <Stack gap="lg" maw={960}>
      <PageHeader
        crumbs={[{ label: 'Settings' }]}
        title="Git"
        description="Connect GitHub with a GitHub App, or save access tokens for any other git host."
      />
      {error && (
        <Alert color="red" variant="light" withCloseButton onClose={() => setParams({})} title="GitHub setup failed">
          {error}
        </Alert>
      )}
      <GitHubApps data={git.data} />
      <Credentials credentials={git.data?.credentials ?? []} />
    </Stack>
  );
}

function GitHubApps({ data }: { data: GitOverview | undefined }) {
  const [org, setOrg] = useState('');
  const [warning, setWarning] = useState('');
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['git'] });

  // GitHub only accepts a manifest as a form POST from the browser.
  const create = useMutation({
    mutationFn: () => api.post<{ action: string; manifest: string; warning?: string }>('/git/github/manifest', { organization: org }),
    onSuccess: (res) => {
      if (res.warning && !warning) {
        setWarning(res.warning);
        return;
      }
      const form = document.createElement('form');
      form.method = 'post';
      form.action = res.action;
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = 'manifest';
      input.value = res.manifest;
      form.appendChild(input);
      document.body.appendChild(form);
      form.submit();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const sync = useMutation({
    mutationFn: (id: number) => api.post(`/git/github/${id}/sync`),
    onSuccess: refresh,
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const remove = useMutation({ mutationFn: (id: number) => api.delete(`/git/github/${id}`), onSuccess: refresh });

  const apps = data?.github_apps ?? [];
  const installs = data?.github_installations ?? [];

  return (
    <Card padding="lg">
      <Stack>
        <div>
          <Group gap="xs">
            <IconBrandGithub size={20} />
            <Text fw={600}>GitHub</Text>
          </Group>
          <Text size="sm" c="dimmed">
            Creates a private GitHub App owned by you, then you install it on the accounts or repos you want to deploy. Private repos
            work without tokens and pushes trigger deployments. GitHub must be able to reach this panel over HTTPS for push webhooks.
          </Text>
        </div>

        {apps.map((app) => (
          <Card key={app.id} padding="md" bg="var(--app-subtle)">
            <Group justify="space-between" wrap="nowrap">
              <div>
                <Anchor href={app.html_url} target="_blank" fw={600}>
                  {app.name}
                </Anchor>
                <Text size="xs" c="dimmed">
                  Owned by {app.owner} · created {timeAgo(app.created_at)}
                </Text>
              </div>
              <Group gap={4}>
                <Button size="xs" variant="default" component="a" href={`${app.html_url}/installations/new`}>
                  Install on an account
                </Button>
                <Tooltip label="Refresh installations from GitHub">
                  <ActionIcon onClick={() => sync.mutate(app.id)} loading={sync.isPending && sync.variables === app.id}>
                    <IconRefresh size={16} />
                  </ActionIcon>
                </Tooltip>
                <Tooltip label="Remove from Moonshed">
                  <ActionIcon
                    color="red"
                    onClick={() =>
                      modals.openConfirmModal({
                        title: `Remove ${app.name}?`,
                        children: (
                          <Text size="sm">
                            Apps using it will no longer be able to clone. Delete the app on GitHub too, from its settings page.
                          </Text>
                        ),
                        labels: { confirm: 'Remove', cancel: 'Cancel' },
                        confirmProps: { color: 'red' },
                        onConfirm: () => remove.mutate(app.id),
                      })
                    }
                  >
                    <IconTrash size={16} />
                  </ActionIcon>
                </Tooltip>
              </Group>
            </Group>
            <Group gap="xs" mt="sm">
              {installs.filter((i) => i.github_app_id === app.id).length === 0 ? (
                <Text size="sm" c="dimmed">
                  Not installed anywhere yet.
                </Text>
              ) : (
                installs
                  .filter((i) => i.github_app_id === app.id)
                  .map((i) => (
                    <Badge key={i.id} variant="default" fw={500}>
                      {i.account} · {i.account_type.toLowerCase()}
                    </Badge>
                  ))
              )}
            </Group>
          </Card>
        ))}

        {warning && (
          <Alert color="orange" variant="light" title="Before you continue">
            {warning}
          </Alert>
        )}
        <Group align="flex-end">
          <TextInput
            label="Organization (optional)"
            description="Leave empty to create the app on your personal account"
            placeholder="my-org"
            value={org}
            onChange={(e) => setOrg(e.currentTarget.value)}
            style={{ flex: 1 }}
          />
          <Button leftSection={<IconBrandGithub size={16} />} onClick={() => create.mutate()} loading={create.isPending}>
            {warning ? 'Continue anyway' : apps.length ? 'Create another GitHub App' : 'Connect GitHub'}
          </Button>
        </Group>
      </Stack>
    </Card>
  );
}

const hostHints: Record<string, string> = {
  'gitlab.com': 'oauth2',
  'github.com': 'x-access-token',
  'bitbucket.org': 'x-token-auth',
};

function Credentials({ credentials }: { credentials: GitCredential[] }) {
  const [editing, setEditing] = useState<GitCredential | 'new' | null>(null);
  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/git/credentials/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['git'] }),
  });

  return (
    <Card padding={0}>
      <Group justify="space-between" p="lg">
        <div>
          <Text fw={600}>Access tokens</Text>
          <Text size="sm" c="dimmed">
            For private repos on GitLab, Bitbucket, Gitea or anywhere else. Apps pick one of these when they use a Git URL.
          </Text>
        </div>
        <Button variant="default" leftSection={<IconPlus size={16} />} onClick={() => setEditing('new')}>
          Add token
        </Button>
      </Group>
      {credentials.length > 0 && (
        <Table>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Name</Table.Th>
              <Table.Th>Host</Table.Th>
              <Table.Th>Username</Table.Th>
              <Table.Th>Added</Table.Th>
              <Table.Th />
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {credentials.map((c) => (
              <Table.Tr key={c.id}>
                <Table.Td fw={500}>{c.name}</Table.Td>
                <Table.Td>{c.host}</Table.Td>
                <Table.Td c="dimmed">{c.username}</Table.Td>
                <Table.Td c="dimmed">{timeAgo(c.created_at)}</Table.Td>
                <Table.Td>
                  <Group gap={4} justify="flex-end" wrap="nowrap">
                    <ActionIcon onClick={() => setEditing(c)}>
                      <IconPencil size={16} />
                    </ActionIcon>
                    <ActionIcon
                      color="red"
                      onClick={() =>
                        modals.openConfirmModal({
                          title: `Delete ${c.name}?`,
                          children: <Text size="sm">Apps using it will clone without a token, which fails for private repos.</Text>,
                          labels: { confirm: 'Delete', cancel: 'Cancel' },
                          confirmProps: { color: 'red' },
                          onConfirm: () => remove.mutate(c.id),
                        })
                      }
                    >
                      <IconTrash size={16} />
                    </ActionIcon>
                  </Group>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
      <CredentialModal credential={editing} onClose={() => setEditing(null)} />
    </Card>
  );
}

function CredentialModal({ credential, onClose }: { credential: GitCredential | 'new' | null; onClose: () => void }) {
  const isNew = credential === 'new';
  const [form, setForm] = useState({ name: '', host: 'gitlab.com', username: 'oauth2', token: '' });
  const [loaded, setLoaded] = useState<GitCredential | 'new' | null>(null);
  if (credential !== loaded) {
    setLoaded(credential);
    setForm(
      credential && credential !== 'new'
        ? { name: credential.name, host: credential.host, username: credential.username, token: '' }
        : { name: '', host: 'gitlab.com', username: 'oauth2', token: '' },
    );
  }
  const save = useMutation({
    mutationFn: () => {
      const body = { ...form, token: form.token || null };
      return isNew ? api.post('/git/credentials', body) : api.put(`/git/credentials/${(credential as GitCredential).id}`, body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['git'] });
      onClose();
    },
  });

  return (
    <Modal opened={credential !== null} onClose={onClose} title={isNew ? 'Add access token' : 'Edit access token'}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Stack>
          {save.error && (
            <Alert color="red" variant="light">
              {save.error.message}
            </Alert>
          )}
          <TextInput label="Name" placeholder="GitLab work" value={form.name} onChange={(e) => setForm({ ...form, name: e.currentTarget.value })} required data-autofocus />
          <TextInput
            label="Host"
            value={form.host}
            onChange={(e) => {
              const host = e.currentTarget.value;
              setForm({ ...form, host, username: hostHints[host] ?? form.username });
            }}
            required
          />
          <TextInput
            label="Username"
            description="GitLab: oauth2 · GitHub: x-access-token · Bitbucket: x-token-auth · others: your username"
            value={form.username}
            onChange={(e) => setForm({ ...form, username: e.currentTarget.value })}
            required
          />
          <PasswordInput
            label="Token"
            description="Read access to repositories is enough"
            placeholder={isNew ? '' : '•••••••• (unchanged)'}
            value={form.token}
            onChange={(e) => setForm({ ...form, token: e.currentTarget.value })}
            required={isNew}
          />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={save.isPending}>
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
