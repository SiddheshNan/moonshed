import { useState } from 'react';
import { Card, Group, Select, Stack, Table, Text, TextInput } from '@mantine/core';
import { IconSearch } from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router';

import { api } from '../api';
import { PageHeader } from '../components/PageHeader';
import { AppStatusBadge, deployLabels } from '../components/StatusBadge';
import { TimeAgo } from '../components/TimeAgo';
import { appPath } from '../paths';
import { byProblem } from '../problems';
import type { App, Project } from '../types';

export function AppsPage() {
  const navigate = useNavigate();
  const [query, setQuery] = useState('');
  const [project, setProject] = useState<string | null>(null);
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps'), refetchInterval: 5000 });
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects') });
  const projectName = new Map(projects.data?.map((p) => [p.id, p.name]));

  const q = query.trim().toLowerCase();
  const rows = byProblem(apps.data ?? []).filter(
    ({ app }) => (!project || app.project_id === project) && (!q || app.name.includes(q) || app.domains.some((d) => d.host.includes(q))),
  );

  return (
    <Stack gap="lg">
      <PageHeader title="Apps" />
      <Group>
        <TextInput
          placeholder="Search by name or domain…"
          leftSection={<IconSearch size={16} />}
          value={query}
          onChange={(e) => setQuery(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <Select
          placeholder="All projects"
          data={(projects.data ?? []).map((p) => ({ value: p.id, label: p.name }))}
          value={project}
          onChange={setProject}
          clearable
          w={200}
        />
      </Group>
      <Card padding={0}>
        <Table.ScrollContainer minWidth={640}>
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>App</Table.Th>
                <Table.Th>Project</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>Last deployment</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {rows.map(({ app, problem }) => (
                <Table.Tr key={app.id} className="row-link" onClick={() => navigate(appPath(app.project_id, app.id))}>
                  <Table.Td maw={320}>
                    <Text size="sm" fw={500} truncate>
                      {app.name}
                    </Text>
                    {problem && (
                      <Text size="xs" c="red" truncate title={problem}>
                        {problem}
                      </Text>
                    )}
                  </Table.Td>
                  <Table.Td c="dimmed">{projectName.get(app.project_id)}</Table.Td>
                  <Table.Td>
                    <AppStatusBadge app={app} />
                  </Table.Td>
                  <Table.Td c="dimmed" className="nowrap">
                    {app.last_deployment ? <TimeAgo iso={app.last_deployment.created_at} prefix={`${deployLabels[app.last_deployment.status]} · `} /> : 'Never'}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        {apps.data && rows.length === 0 && (
          <Text c="dimmed" size="sm" p="lg">
            {apps.data.length === 0 ? 'No apps yet.' : 'No apps match.'}
          </Text>
        )}
      </Card>
    </Stack>
  );
}
