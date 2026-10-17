import { useState } from 'react';
import { ActionIcon, Button, Card, Group, Modal, PasswordInput, Stack, Table, Text, TextInput } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconTrash } from '@tabler/icons-react';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import { timeAgo } from '../../format';
import type { User } from '../../types';

export function UsersPage() {
  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Users" description="Every user is an admin." />
      <Users />
    </Stack>
  );
}

function Users() {
  const users = useQuery({ queryKey: ['users'], queryFn: () => api.get<User[]>('/users') });
  const me = useQuery({ queryKey: ['auth'], queryFn: () => api.get<{ user?: User }>('/auth/status') });
  const [adding, setAdding] = useState(false);
  const [passwordFor, setPasswordFor] = useState<User | null>(null);
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['users'] });
  const reset = () => {
    setAdding(false);
    setPasswordFor(null);
    setUsername('');
    setPassword('');
  };
  const create = useMutation({
    mutationFn: () => api.post('/users', { username, password }),
    onSuccess: () => {
      refresh();
      reset();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const changePassword = useMutation({
    mutationFn: () => api.put(`/users/${passwordFor!.id}/password`, { password }),
    onSuccess: () => {
      notifications.show({ color: 'green', message: 'Password changed' });
      reset();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/users/${id}`),
    onSuccess: refresh,
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Card padding={0}>
      <Group justify="space-between" p="md">
        <Text fw={600}>All users</Text>
        <Button variant="default" onClick={() => setAdding(true)}>
          Add user
        </Button>
      </Group>
      <Table verticalSpacing="sm">
        <Table.Tbody>
          {users.data?.map((u) => (
            <Table.Tr key={u.id}>
              <Table.Td fw={500}>
                {u.username}
                {u.id === me.data?.user?.id && (
                  <Text span c="dimmed" fw={400} size="sm">
                    {' '}
                    (you)
                  </Text>
                )}
              </Table.Td>
              <Table.Td c="dimmed">added {timeAgo(u.created_at)}</Table.Td>
              <Table.Td>
                <Group gap="xs" justify="flex-end">
                  <Button size="xs" variant="subtle" onClick={() => setPasswordFor(u)}>
                    Change password
                  </Button>
                  {u.id !== me.data?.user?.id && (
                    <ActionIcon
                      variant="subtle"
                      color="red"
                      onClick={() =>
                        modals.openConfirmModal({
                          title: `Delete ${u.username}?`,
                          labels: { confirm: 'Delete', cancel: 'Cancel' },
                          confirmProps: { color: 'red' },
                          onConfirm: () => remove.mutate(u.id),
                        })
                      }
                    >
                      <IconTrash size={16} />
                    </ActionIcon>
                  )}
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>

      <Modal opened={adding || passwordFor !== null} onClose={reset} title={adding ? 'Add user' : `Change password for ${passwordFor?.username}`}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (adding) create.mutate();
            else changePassword.mutate();
          }}
        >
          <Stack>
            {adding && <TextInput label="Username" value={username} onChange={(e) => setUsername(e.currentTarget.value)} required data-autofocus />}
            <PasswordInput
              label="Password"
              description="At least 8 characters"
              value={password}
              onChange={(e) => setPassword(e.currentTarget.value)}
              required
            />
            <Group justify="flex-end">
              <Button type="submit" loading={create.isPending || changePassword.isPending}>
                Save
              </Button>
            </Group>
          </Stack>
        </form>
      </Modal>
    </Card>
  );
}
