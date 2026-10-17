import { useEffect, useState } from 'react';
import { Alert, Anchor, Button, Card, Code, Group, NumberInput, SimpleGrid, Stack, Text, TextInput } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api, queryClient } from '../../api';
import {
  appendCertificate,
  CertificateFields,
  CertificateSummary,
  certificateReady,
  emptyCertificate,
  type CertificateValue,
} from '../../components/Certificate';
import { PageHeader } from '../../components/PageHeader';
import { TLSPicker } from '../../components/TLSPicker';
import type { Backup, Settings, SettingsResponse } from '../../types';
import { BackupTable } from '../app/Backups';

export function GeneralSettingsPage() {
  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="General" description="Panel access and platform backups." />
      <GeneralSettings />
      <PlatformBackups />
    </Stack>
  );
}

function panelURL(addr: string): { url: string; local: boolean } {
  const i = addr.lastIndexOf(':');
  const host = addr.slice(0, i);
  const port = addr.slice(i + 1);
  const local = host === '127.0.0.1' || host === 'localhost' || host === '::1';
  return { url: `http://${host && !host.startsWith('0.0.0.0') ? host : location.hostname}:${port}`, local };
}

function GeneralSettings() {
  const query = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const [form, setForm] = useState<Settings | null>(null);
  const [cert, setCert] = useState<CertificateValue>(emptyCertificate);
  useEffect(() => {
    if (query.data && !form) setForm(query.data.settings);
  }, [query.data, form]);

  const save = useMutation({
    mutationFn: async (s: Settings) => {
      if (s.panel_domain && s.panel_tls === 'custom' && certificateReady(cert)) {
        const upload = new FormData();
        upload.append('host', s.panel_domain);
        appendCertificate(upload, cert);
        await api.post('/settings/panel-certificate', upload);
      }
      // The backup schedule is saved separately, so keep whatever is stored.
      const stored = queryClient.getQueryData<SettingsResponse>(['settings'])?.settings;
      return api.put<SettingsResponse>('/settings', { ...s, backup_cron: stored?.backup_cron ?? s.backup_cron, backup_keep: stored?.backup_keep ?? s.backup_keep });
    },
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      queryClient.invalidateQueries({ queryKey: ['server'] });
      setCert(emptyCertificate);
      if (res.error) {
        notifications.show({ color: 'red', title: 'Proxy could not start', message: res.error });
      } else {
        notifications.show({ color: 'green', message: 'Settings saved' });
      }
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  if (!form || !query.data) return null;
  const { panel_addr, panel_certificate } = query.data;
  const panel = panelURL(panel_addr);
  const set = <K extends keyof Settings>(key: K, value: Settings[K]) => setForm({ ...form, [key]: value });
  const customPanelCert = !!form.panel_domain && form.panel_tls === 'custom';
  const savedCustomCert = query.data.settings.panel_domain === form.panel_domain && panel_certificate;

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate(form);
      }}
    >
      <Stack gap="lg">
        <Card>
          <Stack>
            <div>
              <Text fw={600}>Panel access</Text>
              <Text size="sm" c="dimmed">
                The panel is always served on its own port, whatever the proxy is doing:{' '}
                <Anchor href={panel.url} size="sm">
                  {panel.url}
                </Anchor>
                {panel.local ? (
                  <>
                    {' '}
                    (this machine only). Set <Code>panel_addr: ":5050"</Code> in config.yaml to reach it from the network.
                  </>
                ) : (
                  <>
                    . Set <Code>panel_addr: "127.0.0.1:5050"</Code> in config.yaml to keep the panel off the network.
                  </>
                )}
              </Text>
            </div>
            <SimpleGrid cols={{ base: 1, sm: 2 }}>
              <TextInput
                label="Panel domain"
                description="Also serve the panel on a domain through the reverse proxy (optional)"
                placeholder="panel.example.com"
                value={form.panel_domain}
                onChange={(e) => set('panel_domain', e.currentTarget.value)}
              />
              <TLSPicker value={form.panel_tls} onChange={(v) => set('panel_tls', v)} disabled={!form.panel_domain} />
            </SimpleGrid>
            {form.panel_domain && !form.proxy_enabled && (
              <Alert color="orange" variant="light">
                The panel domain only works while the reverse proxy is enabled. Turn it on under{' '}
                <Anchor component={Link} to="/settings/proxy" size="sm">
                  Settings › Proxy
                </Anchor>
                .
              </Alert>
            )}
            {customPanelCert && (
              <Stack gap="xs">
                <Text size="sm" fw={500}>
                  Certificate for {form.panel_domain}
                </Text>
                {savedCustomCert && <CertificateSummary cert={panel_certificate} />}
                <CertificateFields value={cert} onChange={setCert} />
                {savedCustomCert && (
                  <Text size="xs" c="dimmed">
                    Leave the files empty to keep the current certificate.
                  </Text>
                )}
              </Stack>
            )}
          </Stack>
        </Card>

        <Group justify="flex-end">
          <Button type="submit" loading={save.isPending}>
            Save settings
          </Button>
        </Group>
      </Stack>
    </form>
  );
}

function PlatformBackups() {
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const backups = useQuery({ queryKey: ['backups', 'platform'], queryFn: () => api.get<Backup[]>('/platform/backups') });
  const [cron, setCron] = useState<string | null>(null);
  const [keep, setKeep] = useState<number | null>(null);
  const create = useMutation({
    mutationFn: () => api.post('/platform/backups'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups', 'platform'] }),
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const stored = settings.data?.settings;
  const save = useMutation({
    mutationFn: () => api.put<SettingsResponse>('/settings', { ...stored, backup_cron: cron ?? stored?.backup_cron, backup_keep: keep ?? stored?.backup_keep }),
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      notifications.show({ color: 'green', message: 'Schedule saved' });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Card padding={0}>
      <Group justify="space-between" p="md">
        <div>
          <Text fw={600}>Platform backups</Text>
          <Text size="sm" c="dimmed">
            The platform database and its encryption key. To restore, stop the platform and extract the archive into{' '}
            <Code>runtime/data/</Code>.
          </Text>
        </div>
        <Button variant="default" onClick={() => create.mutate()} loading={create.isPending}>
          Back up now
        </Button>
      </Group>
      {stored && (
        <Group align="flex-end" px="md" pb="md">
          <TextInput
            label="Schedule"
            description="Cron, server time. Empty = off."
            placeholder="0 2 * * *"
            value={cron ?? stored.backup_cron}
            onChange={(e) => setCron(e.currentTarget.value)}
            w={260}
          />
          <NumberInput label="Keep" min={1} value={keep ?? stored.backup_keep} onChange={(v) => setKeep(Number(v) || 1)} w={120} />
          <Button variant="default" onClick={() => save.mutate()} loading={save.isPending}>
            Save
          </Button>
        </Group>
      )}
      <BackupTable backups={backups.data ?? []} restorable={false} />
    </Card>
  );
}
