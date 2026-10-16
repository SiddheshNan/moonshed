import { Badge, FileInput, Group, PasswordInput, SegmentedControl, SimpleGrid, Stack, Text } from '@mantine/core';

import { formatDateTime } from '../format';
import type { CertInfo } from '../types';

export interface CertificateValue {
  kind: 'pem' | 'pfx';
  cert: File | null;
  key: File | null;
  pfx: File | null;
  password: string;
}

export const emptyCertificate: CertificateValue = { kind: 'pem', cert: null, key: null, pfx: null, password: '' };

export function certificateReady(v: CertificateValue): boolean {
  return v.kind === 'pem' ? !!(v.cert && v.key) : !!v.pfx;
}

export function appendCertificate(form: FormData, v: CertificateValue) {
  if (v.kind === 'pem') {
    form.append('cert', v.cert!);
    form.append('key', v.key!);
  } else {
    form.append('pfx', v.pfx!);
    form.append('password', v.password);
  }
}

export function CertificateFields({ value, onChange }: { value: CertificateValue; onChange: (v: CertificateValue) => void }) {
  const set = (patch: Partial<CertificateValue>) => onChange({ ...value, ...patch });
  return (
    <Stack gap="sm">
      <SegmentedControl
        size="xs"
        w="fit-content"
        value={value.kind}
        onChange={(kind) => set({ kind: kind as CertificateValue['kind'] })}
        data={[
          { value: 'pem', label: 'PEM files' },
          { value: 'pfx', label: 'PFX / P12' },
        ]}
      />
      {value.kind === 'pem' ? (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <FileInput label="Certificate (with chain)" placeholder=".crt / .pem" value={value.cert} onChange={(cert) => set({ cert })} clearable />
          <FileInput label="Private key" placeholder=".key" value={value.key} onChange={(key) => set({ key })} clearable />
        </SimpleGrid>
      ) : (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <FileInput label="PFX file" placeholder=".pfx / .p12" value={value.pfx} onChange={(pfx) => set({ pfx })} clearable />
          <PasswordInput label="PFX password" value={value.password} onChange={(e) => set({ password: e.currentTarget.value })} />
        </SimpleGrid>
      )}
    </Stack>
  );
}

export function CertificateSummary({ cert }: { cert: CertInfo | null | undefined }) {
  if (!cert) {
    return (
      <Text size="xs" c="red">
        No certificate uploaded
      </Text>
    );
  }
  const days = Math.floor((new Date(cert.not_after).getTime() - Date.now()) / 86_400_000);
  const color = days < 0 ? 'red' : days < 21 ? 'orange' : 'green';
  return (
    <Group gap="xs" wrap="nowrap">
      <Badge size="sm" variant="light" color={color}>
        {days < 0 ? 'expired' : `${days}d left`}
      </Badge>
      <Text size="xs" c="dimmed" truncate>
        {cert.issuer} · expires {formatDateTime(cert.not_after)}
      </Text>
    </Group>
  );
}
