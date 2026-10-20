import { ActionIcon, Badge, Button, FileButton, Group, Input, PasswordInput, SegmentedControl, SimpleGrid, Stack, Text } from '@mantine/core';
import { IconUpload, IconX } from '@tabler/icons-react';

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
          <FilePick label="Certificate (with chain)" hint=".crt or .pem" accept=".crt,.pem,.cer" value={value.cert} onChange={(cert) => set({ cert })} />
          <FilePick label="Private key" hint=".key or .pem" accept=".key,.pem" value={value.key} onChange={(key) => set({ key })} />
        </SimpleGrid>
      ) : (
        <SimpleGrid cols={{ base: 1, sm: 2 }}>
          <FilePick label="PFX file" hint=".pfx or .p12" accept=".pfx,.p12" value={value.pfx} onChange={(pfx) => set({ pfx })} />
          <PasswordInput label="PFX password" value={value.password} onChange={(e) => set({ password: e.currentTarget.value })} />
        </SimpleGrid>
      )}
    </Stack>
  );
}

// FilePick is a labelled "Choose file" button with the picked file's name beside it.
function FilePick({
  label,
  hint,
  accept,
  value,
  onChange,
}: {
  label: string;
  hint: string;
  accept: string;
  value: File | null;
  onChange: (f: File | null) => void;
}) {
  return (
    <Input.Wrapper label={label}>
      <Group gap="sm" wrap="nowrap" mih={36}>
        <FileButton onChange={onChange} accept={accept}>
          {(props) => (
            <Button {...props} variant="default" leftSection={<IconUpload size={16} />} style={{ flexShrink: 0 }}>
              {value ? 'Change' : 'Choose file'}
            </Button>
          )}
        </FileButton>
        <Text size="sm" c={value ? undefined : 'dimmed'} truncate>
          {value ? value.name : hint}
        </Text>
        {value && (
          <ActionIcon variant="subtle" color="gray" onClick={() => onChange(null)} aria-label={`Remove ${label}`}>
            <IconX size={14} />
          </ActionIcon>
        )}
      </Group>
    </Input.Wrapper>
  );
}

// CertificateSummary shows expiry; missing replaces the "No certificate uploaded" warning,
// e.g. for a Let's Encrypt certificate that simply hasn't been issued yet.
export function CertificateSummary({ cert, missing }: { cert: CertInfo | null | undefined; missing?: string }) {
  if (!cert) {
    return (
      <Text size="xs" c={missing ? 'dimmed' : 'red'}>
        {missing ?? 'No certificate uploaded'}
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
