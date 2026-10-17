import { useState } from 'react';
import { Alert, Badge, Button, Card, Center, Loader, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconPencil, IconPlus, IconSearch, IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router';

import { api, queryClient } from '../api';
import { PageHeader } from '../components/PageHeader';
import { AppStatusBadge } from '../components/StatusBadge';
import { formatBytes, formatPercent, runtimeLabel, sourceLabel } from '../format';
import { appPath } from '../paths';
import type { App, Project } from '../types';
import { NewAppModal } from './NewAppModal';
import { ProjectModal } from './Projects';

export function ProjectPage() {
  const id = useParams().id ?? '';
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Project | null>(null);
  const data = useQuery({
    queryKey: ['project', id],
    queryFn: () => api.get<{ project: Project; apps: App[] }>(`/projects/${id}`),
    refetchInterval: 5000,
  });

  const remove = useMutation({
    mutationFn: () => api.delete(`/projects/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      navigate('/projects');
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  if (data.isPending) {
    return (
      <Center h={300}>
        <Loader />
      </Center>
    );
  }
  if (data.error || !data.data) {
    return <Alert color="red">{data.error?.message ?? 'Project not found'}</Alert>;
  }

  const { project, apps } = data.data;
  const list = apps.filter((a) => a.name.includes(query.toLowerCase()));

  return (
    <Stack gap="lg">
      <PageHeader
        crumbs={[{ label: 'Projects', to: '/projects' }, { label: project.name }]}
        title={project.name}
        description={project.description || undefined}
        actions={
          <>
            <Button leftSection={<IconPlus size={16} />} onClick={() => setCreating(true)}>
              New app
            </Button>
            <Button variant="default" leftSection={<IconPencil size={16} />} onClick={() => setEditing(project)}>
              Edit
            </Button>
            <Tooltip label="Move or delete its apps first" disabled={apps.length === 0}>
              <Button
                variant="default"
                color="red"
                leftSection={<IconTrash size={16} />}
                disabled={apps.length > 0}
                onClick={() =>
                  modals.openConfirmModal({
                    title: `Delete ${project.name}?`,
                    labels: { confirm: 'Delete', cancel: 'Cancel' },
                    confirmProps: { color: 'red' },
                    onConfirm: () => remove.mutate(),
                  })
                }
              >
                Delete
              </Button>
            </Tooltip>
          </>
        }
      />

      {apps.length > 8 && (
        <TextInput placeholder="Filter apps…" leftSection={<IconSearch size={16} />} value={query} onChange={(e) => setQuery(e.currentTarget.value)} />
      )}

      <Card padding={0}>
        {apps.length === 0 ? (
          <Text c="dimmed" size="sm" p="lg">
            No apps in this project yet.
          </Text>
        ) : (
          <Table.ScrollContainer minWidth={640}>
            <Table highlightOnHover>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Name</Table.Th>
                  <Table.Th>Status</Table.Th>
                  <Table.Th>Runtime</Table.Th>
                  <Table.Th>Source</Table.Th>
                  <Table.Th>CPU</Table.Th>
                  <Table.Th>Memory</Table.Th>
                  <Table.Th>Domains</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {list.map((app) => {
                  const running = app.status.state === 'running';
                  return (
                    <Table.Tr key={app.id} className="row-link" onClick={() => navigate(appPath(app.project_id, app.id))}>
                      <Table.Td fw={600} className="nowrap">
                        {app.name}
                      </Table.Td>
                      <Table.Td>
                        <AppStatusBadge app={app} />
                      </Table.Td>
                      <Table.Td>
                        {app.runtime && (
                          <Badge variant="default" fw={500}>
                            {runtimeLabel(app.runtime)}
                          </Badge>
                        )}
                      </Table.Td>
                      <Table.Td c="dimmed" className="nowrap">
                        {sourceLabel(app)}
                      </Table.Td>
                      <Table.Td>{running && app.stats.processes ? formatPercent(app.stats.cpu) : '—'}</Table.Td>
                      <Table.Td>{running && app.stats.processes ? formatBytes(app.stats.memory) : '—'}</Table.Td>
                      <Table.Td>
                        <Text size="sm" c="dimmed" truncate maw={220}>
                          {app.domains.map((d) => d.host).join(', ') || '—'}
                        </Text>
                      </Table.Td>
                    </Table.Tr>
                  );
                })}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </Card>

      <NewAppModal opened={creating} onClose={() => setCreating(false)} projectId={id} />
      <ProjectModal project={editing} onClose={() => setEditing(null)} />
    </Stack>
  );
}
