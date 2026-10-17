import { useState } from 'react';
import { ActionIcon, Alert, Button, Card, Code, Group, Stack, Table, Text, TextInput } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import { timeAgo } from '../../format';
import type { APIToken } from '../../types';

export function TokensPage() {
  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="API tokens" />
      <Tokens />
    </Stack>
  );
}

function Tokens() {
  const tokens = useQuery({ queryKey: ['tokens'], queryFn: () => api.get<APIToken[]>('/tokens') });
  const [name, setName] = useState('');
  const [created, setCreated] = useState<string | null>(null);
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['tokens'] });
  const create = useMutation({
    mutationFn: () => api.post<{ token: string }>('/tokens', { name }),
    onSuccess: (res) => {
      setCreated(res.token);
      setName('');
      refresh();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const remove = useMutation({ mutationFn: (id: string) => api.delete(`/tokens/${id}`), onSuccess: refresh });

  return (
    <Card padding={0}>
      <Stack p="md" gap="sm">
        <div>
          <Text fw={600}>Tokens</Text>
          <Text size="sm" c="dimmed">
            Send as <Code>Authorization: Bearer &lt;token&gt;</Code>.
          </Text>
        </div>
        {created && (
          <Alert color="green" variant="light" title="Copy this token now, it won't be shown again" withCloseButton onClose={() => setCreated(null)}>
            <Code style={{ wordBreak: 'break-all' }}>{created}</Code>
          </Alert>
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <Group>
            <TextInput placeholder="Token name, e.g. github-actions" value={name} onChange={(e) => setName(e.currentTarget.value)} style={{ flex: 1 }} required />
            <Button type="submit" variant="default" loading={create.isPending}>
              Create token
            </Button>
          </Group>
        </form>
      </Stack>
      <Table verticalSpacing="sm">
        <Table.Tbody>
          {tokens.data?.map((t) => (
            <Table.Tr key={t.id}>
              <Table.Td fw={500}>{t.name}</Table.Td>
              <Table.Td c="dimmed">created {timeAgo(t.created_at)}</Table.Td>
              <Table.Td c="dimmed">{t.last_used_at ? `used ${timeAgo(t.last_used_at)}` : 'never used'}</Table.Td>
              <Table.Td>
                <Group justify="flex-end">
                  <ActionIcon variant="subtle" color="red" onClick={() => remove.mutate(t.id)}>
                    <IconTrash size={16} />
                  </ActionIcon>
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Card>
  );
}
