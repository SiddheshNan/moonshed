import { useState, type ReactNode } from 'react';
import { ActionIcon, Button, Code, CopyButton, Group, Modal, Stack, Text, TextInput, Tooltip } from '@mantine/core';
import { IconCheck, IconCopy } from '@tabler/icons-react';

// ConfirmDelete asks for the exact name before deleting something that can't be restored.
export function ConfirmDelete({
  opened,
  onClose,
  title,
  name,
  children,
  loading,
  onConfirm,
}: {
  opened: boolean;
  onClose: () => void;
  title: string;
  name: string;
  children: ReactNode;
  loading?: boolean;
  onConfirm: () => void;
}) {
  const [typed, setTyped] = useState('');
  const close = () => {
    setTyped('');
    onClose();
  };
  const matches = typed.trim() === name;

  return (
    <Modal opened={opened} onClose={close} title={title}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (matches) onConfirm();
        }}
      >
        <Stack>
          <Text size="sm">{children}</Text>
          <Stack gap={6}>
            <Group gap={6} wrap="nowrap">
              <Text size="sm">Type</Text>
              <Code fz="sm">{name}</Code>
              <CopyButton value={name}>
                {({ copied, copy }) => (
                  <Tooltip label={copied ? 'Copied' : 'Copy'}>
                    <ActionIcon size="sm" onClick={copy} aria-label="Copy name">
                      {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                    </ActionIcon>
                  </Tooltip>
                )}
              </CopyButton>
              <Text size="sm">to confirm.</Text>
            </Group>
            <TextInput value={typed} onChange={(e) => setTyped(e.currentTarget.value)} placeholder={name} autoComplete="off" data-autofocus />
          </Stack>
          <Group justify="flex-end">
            <Button variant="default" onClick={close}>
              Cancel
            </Button>
            <Button type="submit" color="red" disabled={!matches} loading={loading}>
              Delete
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  );
}
