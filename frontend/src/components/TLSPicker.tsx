import { SegmentedControl, Stack, Text } from '@mantine/core';

import type { TLSMode } from '../types';

export const tlsLabels: Record<TLSMode, string> = { letsencrypt: "Let's Encrypt", custom: 'Custom certificate', none: 'HTTP only' };

// TLSPicker is a two-step choice: HTTP only or HTTPS, and for HTTPS where the certificate comes from.
export function TLSPicker({ value, onChange, disabled }: { value: TLSMode; onChange: (v: TLSMode) => void; disabled?: boolean }) {
  const https = value !== 'none';
  return (
    <Stack gap={6}>
      <Text size="sm" fw={500}>
        Protocol
      </Text>
      <SegmentedControl
        w="fit-content"
        disabled={disabled}
        value={https ? 'https' : 'http'}
        onChange={(v) => onChange(v === 'http' ? 'none' : 'letsencrypt')}
        data={[
          { value: 'https', label: 'HTTPS' },
          { value: 'http', label: 'HTTP only' },
        ]}
      />
      {https && (
        <>
          <Text size="sm" fw={500} mt={4}>
            Certificate
          </Text>
          <SegmentedControl
            w="fit-content"
            disabled={disabled}
            value={value}
            onChange={(v) => onChange(v as TLSMode)}
            data={[
              { value: 'letsencrypt', label: "Let's Encrypt" },
              { value: 'custom', label: 'Custom certificate' },
            ]}
          />
          <Text size="xs" c="dimmed">
            {value === 'letsencrypt'
              ? 'Issued and renewed automatically. Needs ports 80 and 443 reachable from the internet.'
              : 'Upload your own certificate and key, or a PFX.'}{' '}
            HTTP requests are redirected to HTTPS.
          </Text>
        </>
      )}
    </Stack>
  );
}
