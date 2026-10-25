import { useState } from 'react';
import { useSearchParams } from 'react-router';
import {
  ActionIcon,
  Alert,
  Button,
  Center,
  Code,
  CopyButton,
  Group,
  Paper,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core';
import { IconCheck, IconCopy } from '@tabler/icons-react';

import { api, queryClient } from '../api';
import { Logo } from '../components/Layout';

export function Login({ setup, setupCommand }: { setup: boolean; setupCommand?: string }) {
  const [params] = useSearchParams();
  const token = params.get('token') ?? '';
  const [username, setUsername] = useState(setup ? 'admin' : '');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.post(setup ? '/auth/setup' : '/auth/login', setup ? { username, password, token } : { username, password });
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
          {setup && !token ? (
            <SetupHint command={setupCommand ?? ''} />
          ) : (
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
          )}
        </Paper>
      </Stack>
    </Center>
  );
}

function SetupHint({ command }: { command: string }) {
  return (
    <Stack gap="sm">
      <Text size="sm">
        Open the setup link printed when Moonshed was installed. To print it again, run this on the server:
      </Text>
      <Group gap={6} wrap="nowrap" align="flex-start">
        <Code block style={{ flex: 1, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
          {command}
        </Code>
        <CopyButton value={command}>
          {({ copied, copy }) => (
            <Tooltip label={copied ? 'Copied' : 'Copy'}>
              <ActionIcon variant="default" onClick={copy} aria-label="Copy command">
                {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
              </ActionIcon>
            </Tooltip>
          )}
        </CopyButton>
      </Group>
    </Stack>
  );
}
