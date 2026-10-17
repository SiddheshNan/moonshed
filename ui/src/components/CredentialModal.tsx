import { useState } from 'react';
import { Alert, Button, Group, Modal, PasswordInput, Stack, TextInput } from '@mantine/core';
import { useMutation } from '@tanstack/react-query';

import { api, queryClient } from '../api';
import type { GitCredential } from '../types';

// The user name git expects with a token on hosts that have a fixed one.
const tokenUsers: Record<string, string> = {
  'gitlab.com': 'oauth2',
  'github.com': 'x-access-token',
  'bitbucket.org': 'x-token-auth',
};

type Form = { name: string; host: string; username: string; token: string };

function initialForm(credential: GitCredential | 'new' | null, host: string): Form {
  if (credential && credential !== 'new') return { name: credential.name, host: credential.host, username: credential.username, token: '' };
  return { name: '', host, username: tokenUsers[host] ?? '', token: '' };
}

// CredentialModal adds or edits a git access token. onSaved gets the stored credential.
export function CredentialModal({
  credential,
  host = '',
  onClose,
  onSaved,
}: {
  credential: GitCredential | 'new' | null;
  host?: string;
  onClose: () => void;
  onSaved?: (c: GitCredential) => void;
}) {
  const isNew = credential === 'new';
  const [form, setForm] = useState<Form>(() => initialForm(credential, host));
  const [loaded, setLoaded] = useState(credential);
  if (credential !== loaded) {
    setLoaded(credential);
    setForm(initialForm(credential, host));
  }
  const fixedUser = tokenUsers[form.host.trim().toLowerCase()];

  const save = useMutation({
    mutationFn: () => {
      const body = { ...form, username: fixedUser ?? form.username, token: form.token || null };
      return isNew ? api.post<GitCredential>('/git/credentials', body) : api.put<GitCredential>(`/git/credentials/${(credential as GitCredential).id}`, body);
    },
    onSuccess: (c) => {
      queryClient.invalidateQueries({ queryKey: ['git'] });
      onSaved?.(c);
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
          <TextInput label="Name" placeholder="Work account" value={form.name} onChange={(e) => setForm({ ...form, name: e.currentTarget.value })} required data-autofocus />
          <TextInput label="Host" placeholder="gitlab.com" value={form.host} onChange={(e) => setForm({ ...form, host: e.currentTarget.value })} required />
          {!fixedUser && (
            <TextInput label="Username" placeholder="Your username on that host" value={form.username} onChange={(e) => setForm({ ...form, username: e.currentTarget.value })} required />
          )}
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
