import { useState } from 'react';
import { Alert, Button, Card, Group, Modal, Select, SimpleGrid, Stack, Text, Textarea, TextInput } from '@mantine/core';
import { IconFolder, IconPlus, IconSearch } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api, queryClient } from '../api';
import { PageHeader } from '../components/PageHeader';
import { timeAgo } from '../format';
import type { Project, ProjectSummary } from '../types';

const sorts = [
  { value: 'name', label: 'Name' },
  { value: 'newest', label: 'Newest first' },
  { value: 'apps', label: 'Most apps' },
];

export function ProjectsPage() {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState('name');
  const [editing, setEditing] = useState<Project | 'new' | null>(null);
  const projects = useQuery({ queryKey: ['projects'], queryFn: () => api.get<ProjectSummary[]>('/projects'), refetchInterval: 10000 });

  const list = (projects.data ?? [])
    .filter((p) => p.name.toLowerCase().includes(query.toLowerCase()) || p.description.toLowerCase().includes(query.toLowerCase()))
    .sort((a, b) => {
      if (sort === 'newest') return b.created_at.localeCompare(a.created_at);
      if (sort === 'apps') return b.apps - a.apps;
      return a.name.localeCompare(b.name);
    });

  return (
    <Stack gap="lg">
      <PageHeader
        title="Projects"
        actions={
          <Button leftSection={<IconPlus size={16} />} onClick={() => setEditing('new')}>
            New project
          </Button>
        }
      />
      <Group>
        <TextInput
          placeholder="Filter projects…"
          leftSection={<IconSearch size={16} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <Select data={sorts} value={sort} onChange={(v) => setSort(v ?? 'name')} allowDeselect={false} w={170} />
      </Group>
      <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
        {list.map((p) => (
          <Card key={p.id} padding="lg" className="row-link" onClick={() => navigate(`/projects/${p.id}`)}>
            <Group gap="sm" wrap="nowrap">
              <IconFolder size={20} stroke={1.6} />
              <Text fw={600} truncate>
                {p.name}
              </Text>
            </Group>
            <Text size="sm" c="dimmed" mt={6} lineClamp={2} mih={40}>
              {p.description}
            </Text>
            <Group justify="space-between" mt="md">
              <Text size="xs" c="dimmed">
                Created {timeAgo(p.created_at)}
              </Text>
              <Group gap="xs">
                <Text size="sm" fw={500}>
                  {p.apps} {p.apps === 1 ? 'app' : 'apps'}
                </Text>
                {p.unhealthy > 0 && (
                  <Text size="sm" c="orange" fw={500}>
                    · {p.unhealthy} {p.unhealthy === 1 ? 'needs' : 'need'} attention
                  </Text>
                )}
              </Group>
            </Group>
          </Card>
        ))}
      </SimpleGrid>
      {projects.data && list.length === 0 && (
        <Text c="dimmed" size="sm">
          No projects match.
        </Text>
      )}
      <ProjectModal project={editing} onClose={() => setEditing(null)} />
    </Stack>
  );
}

// ProjectModal creates or edits a project. onSaved gets the stored project.
export function ProjectModal({
  project,
  onClose,
  onSaved,
}: {
  project: Project | 'new' | null;
  onClose: () => void;
  onSaved?: (p: Project) => void;
}) {
  const isNew = project === 'new';
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [loaded, setLoaded] = useState<Project | 'new' | null>(null);
  if (project !== loaded) {
    setLoaded(project);
    setName(project && project !== 'new' ? project.name : '');
    setDescription(project && project !== 'new' ? project.description : '');
  }

  const save = useMutation({
    mutationFn: () =>
      isNew ? api.post<Project>('/projects', { name, description }) : api.put<Project>(`/projects/${(project as Project).id}`, { name, description }),
    onSuccess: (p) => {
      queryClient.invalidateQueries({ queryKey: ['projects'] });
      queryClient.invalidateQueries({ queryKey: ['projects-list'] });
      queryClient.invalidateQueries({ queryKey: ['project'] });
      onSaved?.(p);
      onClose();
    },
  });

  return (
    <Modal opened={project !== null} onClose={onClose} title={isNew ? 'New project' : 'Edit project'}>
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
          <TextInput label="Name" value={name} onChange={(e) => setName(e.currentTarget.value)} required data-autofocus />
          <Textarea label="Description (optional)" value={description} onChange={(e) => setDescription(e.currentTarget.value)} autosize minRows={2} />
          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={save.isPending}>
              {isNew ? 'Create project' : 'Save'}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
