import { useState, type ReactNode } from 'react';
import { Anchor, Button, Group, NumberInput, Popover, Text } from '@mantine/core';

// InlineSetting shows a setting as a sentence with a "Change" link that edits the number in place.
export function InlineSetting({
  children,
  label,
  value,
  min = 1,
  saving,
  onSave,
}: {
  children: ReactNode;
  label: string;
  value: number;
  min?: number;
  saving?: boolean;
  onSave: (v: number, done: () => void) => void;
}) {
  const [opened, setOpened] = useState(false);
  const [draft, setDraft] = useState<number | string>(value);
  return (
    <Group gap={6} wrap="nowrap">
      <Text size="sm" c="dimmed">
        {children}
      </Text>
      <Popover opened={opened} onChange={setOpened} position="bottom-end" withArrow trapFocus>
        <Popover.Target>
          <Anchor
            component="button"
            type="button"
            size="sm"
            onClick={() => {
              setDraft(value);
              setOpened((o) => !o);
            }}
          >
            Change
          </Anchor>
        </Popover.Target>
        <Popover.Dropdown>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              onSave(Number(draft) || min, () => setOpened(false));
            }}
          >
            <Group gap="xs" align="flex-end" wrap="nowrap">
              <NumberInput label={label} min={min} value={draft} onChange={setDraft} w={160} data-autofocus />
              <Button type="submit" loading={saving}>
                Save
              </Button>
            </Group>
          </form>
        </Popover.Dropdown>
      </Popover>
    </Group>
  );
}
