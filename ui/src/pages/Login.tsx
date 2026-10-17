import { useState } from 'react';
import { Alert, Button, Center, Paper, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core';

import { api, queryClient } from '../api';
import { Logo } from '../components/Layout';

export function Login({ setup }: { setup: boolean }) {
  const [username, setUsername] = useState(setup ? 'admin' : '');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.post(setup ? '/auth/setup' : '/auth/login', { username, password });
      await queryClient.invalidateQueries({ queryKey: ['auth'] });
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Center mih="100vh" p="md" className="auth-backdrop">
      <div className="auth-grid" />
      <Stack align="center" gap="lg" w={400} maw="100%" pos="relative">
        <Logo size="lg" />
        <Stack gap={6} align="center">
          <Title order={2}>{setup ? 'Set up Moonshed' : 'Welcome back'}</Title>
          <Text size="sm" c="dimmed" ta="center">
            {setup
              ? 'This is a fresh install. The account you create here is the first admin.'
              : 'Sign in to manage your apps.'}
          </Text>
        </Stack>
        <Paper withBorder p="xl" w="100%" bg="var(--app-surface)" style={{ borderColor: 'var(--app-border)' }} shadow="xl">
          <form onSubmit={submit}>
            <Stack>
              {error && (
                <Alert color="red" variant="light">
                  {error}
                </Alert>
              )}
              <TextInput label="Username" value={username} onChange={(e) => setUsername(e.currentTarget.value)} required autoFocus />
              <PasswordInput
                label="Password"
                description={setup ? 'At least 8 characters' : undefined}
                value={password}
                onChange={(e) => setPassword(e.currentTarget.value)}
                required
              />
              <Button type="submit" loading={busy} fullWidth mt="xs">
                {setup ? 'Create account' : 'Sign in'}
              </Button>
            </Stack>
          </form>
        </Paper>
      </Stack>
    </Center>
  );
}
