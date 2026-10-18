import { useState } from 'react';
import { Button, Group, Modal, Select, Stack } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';

import { api } from '../../api';
import type { App, Project } from '../../types';
import { ProjectModal } from '../Projects';
import { useSave } from './Settings';

const newProject = '__new';

// MoveModal moves an app to another project, or to a new one created on the spot.
export function MoveModal({ app, opened, onClose }: { app: App; opened: boolean; onClose: () => void }) {
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects'), enabled: opened });
  const [project, setProject] = useState(app.project_id);
  const [creating, setCreating] = useState(false);
  const save = useSave(app);

  return (
    <Modal opened={opened} onClose={onClose} title={`Move ${app.name}`} size="sm" closeOnEscape={!creating}>
      <Stack>
        <Select
          label="Project"
          data={[...(projects.data ?? []).map((p) => ({ value: p.id, label: p.name })), { value: newProject, label: 'New project…' }]}
          value={project}
          onChange={(v) => (v === newProject ? setCreating(true) : v && setProject(v))}
          allowDeselect={false}
          data-autofocus
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button
            disabled={project === app.project_id}
            loading={save.isPending}
            onClick={() => save.mutate({ project_id: project }, { onSuccess: onClose })}
          >
            Move
          </Button>
        </Group>
      </Stack>
      <ProjectModal project={creating ? 'new' : null} onClose={() => setCreating(false)} onSaved={(p) => setProject(p.id)} />
    </Modal>
  );
}
