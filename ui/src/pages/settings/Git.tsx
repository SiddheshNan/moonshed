import { type ReactNode, useState } from 'react';
import {
  ActionIcon,
  Alert,
  Anchor,
  Badge,
  Button,
  Card,
  Group,
  Radio,
  Stack,
  Table,
  Text,
  TextInput,
  ThemeIcon,
  Tooltip,
} from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconBrandGithub, IconExternalLink, IconPencil, IconPlus, IconRefresh, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Link, useSearchParams } from 'react-router';

import { api, queryClient } from '../../api';
import { CredentialModal } from '../../components/CredentialModal';
import { PageHeader } from '../../components/PageHeader';
import { TimeAgo } from '../../components/TimeAgo';
import type { GitCredential, GitOverview } from '../../types';

export function GitSettingsPage() {
  const [params, setParams] = useSearchParams();
  const error = params.get('error');
  const git = useQuery({ queryKey: ['git'], queryFn: () => api.get<GitOverview>('/git') });

  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Git" />
      {error && (
        <Alert color="red" variant="light" withCloseButton onClose={() => setParams({})} title="GitHub App setup failed">
          {error}
        </Alert>
      )}
      <GitHubApps data={git.data} />
      <Credentials credentials={git.data?.credentials ?? []} />
    </Stack>
  );
}

function GitHubApps({ data }: { data: GitOverview | undefined }) {
  const [adding, setAdding] = useState(false);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['git'] });
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
        <Group justify="space-between" align="flex-start" wrap="nowrap">
          <div>
            <Group gap="xs">
              <IconBrandGithub size={20} />
              <Text fw={600}>GitHub App</Text>
              {data && (
                <Badge variant="light" color={installs.length ? 'green' : 'gray'}>
                  {installs.length ? 'Connected' : apps.length ? 'Not installed' : 'Not connected'}
                </Badge>
              )}
            </Group>
            <Text size="sm" c="dimmed">
              Pick repositories from a list, deploy private repositories without a personal token, and deploy on every push.
            </Text>
          </div>
          {apps.length > 0 && !adding && (
            <Button variant="default" size="xs" leftSection={<IconPlus size={14} />} onClick={() => setAdding(true)} style={{ flexShrink: 0 }}>
              Add another GitHub App
            </Button>
          )}
        </Group>

        {data && apps.length === 0 && <ConnectGitHub />}

        {apps.map((app) => {
          const appInstalls = installs.filter((i) => i.github_app_id === app.id);
          return (
            <Card key={app.id} padding="md" bg="var(--app-subtle)">
              <Stack gap="sm">
                <Group justify="space-between" wrap="nowrap" align="flex-start">
                  <div>
                    <Anchor href={app.html_url} target="_blank" fw={600}>
                      {app.name}
                    </Anchor>
                    <Text size="xs" c="dimmed">
                      GitHub App owned by {app.owner} · <TimeAgo iso={app.created_at} prefix="created " />
                    </Text>
                  </div>
                  <Group gap={4} wrap="nowrap">
                    <Tooltip label="Refresh from GitHub">
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
                <div>
                  <Text size="sm" fw={500} mb={6}>
                    Has access to
                  </Text>
                  {appInstalls.length === 0 ? (
                    <Text size="sm" c="dimmed">
                      No accounts yet. Install it on GitHub and choose which repositories Moonshed may read.
                    </Text>
                  ) : (
                    <Group gap="xs">
                      {appInstalls.map((i) => (
                        <Badge key={i.id} variant="default" fw={500} tt="none">
                          {i.account} · {i.account_type === 'Organization' ? 'organization' : 'personal account'}
                        </Badge>
                      ))}
                    </Group>
                  )}
                </div>
                <Group gap="sm">
                  <Button
                    size="xs"
                    variant={appInstalls.length ? 'default' : 'filled'}
                    component="a"
                    href={`${app.html_url}/installations/new`}
                    rightSection={<IconExternalLink size={14} />}
                  >
                    {appInstalls.length ? 'Install on another account' : 'Install on GitHub'}
                  </Button>
                  {appInstalls.length > 0 && (
                    <Text size="xs" c="dimmed">
                      To change which repositories it can read, configure the installation on GitHub, then refresh here.
                    </Text>
                  )}
                </Group>
              </Stack>
            </Card>
          );
        })}

        {adding && <ConnectGitHub onCancel={() => setAdding(false)} />}
      </Stack>
    </Card>
  );
}

// ConnectGitHub creates a GitHub App through GitHub's manifest flow, owned by the user's account or an organization.
function ConnectGitHub({ onCancel }: { onCancel?: () => void }) {
  const [owner, setOwner] = useState<'personal' | 'org'>('personal');
  const [org, setOrg] = useState('');
  const [warning, setWarning] = useState('');

  // GitHub only accepts a manifest as a form POST from the browser.
  const create = useMutation({
    mutationFn: () =>
      api.post<{ action: string; manifest: string; warning?: string }>('/git/github/manifest', {
        organization: owner === 'org' ? org.trim() : '',
      }),
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

  return (
    <Card withBorder padding="md">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <Stack gap="md">
          <Stack gap={8}>
            <Text size="sm">You'll create a GitHub App under your own account. Moonshed only reads the repositories you give the app access to.</Text>
            <Step n={1} title="Create the app">
              GitHub opens with the app's settings already filled in. Confirm with <b>Create GitHub App</b>.
            </Step>
            <Step n={2} title="Install it">
              Choose the account and the repositories Moonshed may read. You can change this any time.
            </Step>
          </Stack>
          <Radio.Group
            label="Who should own the GitHub App?"
            description="The owner manages its settings on GitHub. Either way, it can be installed on other accounts later."
            value={owner}
            onChange={(v) => {
              setOwner(v as 'personal' | 'org');
              setWarning('');
            }}
          >
            <Stack gap="xs" mt="xs">
              <Radio value="personal" label="My personal GitHub account" />
              <Radio value="org" label="A GitHub organization" />
            </Stack>
          </Radio.Group>
          {owner === 'org' && (
            <TextInput
              label="Organization name"
              description="As in github.com/<name>. You need to be an owner of the organization."
              placeholder="my-org"
              value={org}
              onChange={(e) => setOrg(e.currentTarget.value)}
              required
              maw={360}
              autoFocus
            />
          )}
          {warning && (
            <Alert color="orange" variant="light" title="Before you continue">
              {warning}
            </Alert>
          )}
          <Group gap="xs">
            <Button type="submit" leftSection={<IconBrandGithub size={16} />} rightSection={<IconExternalLink size={14} />} loading={create.isPending}>
              {warning ? 'Continue anyway' : 'Continue on GitHub'}
            </Button>
            {onCancel && (
              <Button variant="subtle" color="gray" onClick={onCancel}>
                Cancel
              </Button>
            )}
          </Group>
        </Stack>
      </form>
    </Card>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: ReactNode }) {
  return (
    <Group gap="sm" wrap="nowrap" align="flex-start">
      <ThemeIcon size={22} radius="xl" variant="light" mt={1}>
        <Text size="xs" fw={700}>
          {n}
        </Text>
      </ThemeIcon>
      <Text size="sm">
        <Text span fw={500}>
          {title}.
        </Text>{' '}
        <Text span c="dimmed">
          {children}
        </Text>
      </Text>
    </Group>
  );
}

function Credentials({ credentials }: { credentials: GitCredential[] }) {
  const [editing, setEditing] = useState<GitCredential | 'new' | null>(null);
  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/git/credentials/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['git'] }),
  });

  return (
    <Card padding={0}>
      <Group justify="space-between" p="lg" wrap="nowrap" align="flex-start">
        <div>
          <Text fw={600}>Other git hosts</Text>
          <Text size="sm" c="dimmed">
            Logins Moonshed uses to clone private repositories from GitLab, Bitbucket, Gitea or any other host: a username and a personal access token
            made on that host. Public repositories need none. These are not{' '}
            <Anchor component={Link} to="/settings/tokens" size="sm">
              API tokens
            </Anchor>
            , which let scripts and CI talk to Moonshed.
          </Text>
        </div>
        <Button variant="default" leftSection={<IconPlus size={16} />} onClick={() => setEditing('new')} style={{ flexShrink: 0 }}>
          Add git login
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
                <Table.Td c="dimmed"><TimeAgo iso={c.created_at} /></Table.Td>
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
                          children: <Text size="sm">Apps using it will clone without a login, which fails for private repositories.</Text>,
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
