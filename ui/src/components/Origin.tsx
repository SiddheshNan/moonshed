import { Anchor, Group, Text } from '@mantine/core';

import type { Script } from '../types';

// seconds reads a Go duration like "30s", "1m0s" or "1h30m0s".
export function seconds(d: string | undefined): number | undefined {
  if (!d) return undefined;
  let total = 0;
  for (const [, n, unit] of d.matchAll(/([\d.]+)(ms|h|m|s)/g)) {
    total += Number(n) * { h: 3600, m: 60, s: 1, ms: 0.001 }[unit as 'h' | 'm' | 's' | 'ms'];
  }
  return total;
}

export const duration = (s: number) => `${s}s`;

// scriptText is the script for the server's OS. moonshed.yaml can set one per OS.
export function scriptText(s: Script | undefined, windows: boolean): string {
  return (windows ? s?.windows : s?.linux) ?? '';
}

export const fileName = 'moonshed.yaml';

// mono sets only the input in monospace, not its label.
export const mono = { input: { fontFamily: 'var(--mantine-font-family-monospace)' } };

// Origin says where a setting's value comes from: changed here (with a way back to the
// file) or moonshed.yaml. Defaults need no label.
export function Origin({ changed, inFile, onReset }: { changed: boolean; inFile: boolean; onReset: () => void }) {
  if (changed) {
    return (
      <Group gap={6} wrap="nowrap">
        <Text size="xs" c="indigo.4">
          Changed here
        </Text>
        <Text size="xs" c="dimmed">
          ·
        </Text>
        <Anchor component="button" type="button" size="xs" c="dimmed" onClick={onReset}>
          {inFile ? `Use ${fileName}` : 'Use default'}
        </Anchor>
      </Group>
    );
  }
  if (inFile) {
    return (
      <Text size="xs" c="dimmed">
        From {fileName}
      </Text>
    );
  }
  return null;
}
