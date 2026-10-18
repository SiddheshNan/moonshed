import { useEffect, useState } from 'react';
import { Anchor, Button, Card, Code, Group, NumberInput, SimpleGrid, Stack, Switch, Text, TextInput } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';

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
import { ProxyOff } from '../../components/ProxyOff';
import { TLSPicker } from '../../components/TLSPicker';
import type { Backup, Settings, SettingsResponse } from '../../types';
import { BackupTable } from '../app/Backups';

export function GeneralSettingsPage() {
  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="General" />
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
      return api.put<SettingsResponse>('/settings', {
        ...s,
        backup_enabled: stored?.backup_enabled ?? s.backup_enabled,
        backup_cron: stored?.backup_cron ?? s.backup_cron,
        backup_keep: stored?.backup_keep ?? s.backup_keep,
      });
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
                Always at{' '}
                <Anchor href={panel.url} size="sm">
                  {panel.url}
                </Anchor>
                {panel.local ? (
                  <>
                    , this machine only. To reach it from the network, set <Code>panel_addr: ":5050"</Code> in <Code>{query.data.config_file}</Code>.
                  </>
                ) : (
                  <>
                    . To keep it off the network, set <Code>panel_addr: "127.0.0.1:5050"</Code> in <Code>{query.data.config_file}</Code>.
                  </>
                )}
              </Text>
            </div>
            <SimpleGrid cols={{ base: 1, sm: 2 }}>
              <TextInput
                label="Panel domain (optional)"
                placeholder="panel.example.com"
                value={form.panel_domain}
                onChange={(e) => set('panel_domain', e.currentTarget.value)}
              />
              <TLSPicker value={form.panel_tls} onChange={(v) => set('panel_tls', v)} disabled={!form.panel_domain} />
            </SimpleGrid>
            {form.panel_domain && !form.proxy_enabled && <ProxyOff>The panel domain needs the reverse proxy.</ProxyOff>}
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
            <Group justify="flex-end">
              <Button type="submit" loading={save.isPending}>
                Save
              </Button>
            </Group>
          </Stack>
        </Card>
      </Stack>
    </form>
  );
}

function PlatformBackups() {
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const backups = useQuery({ queryKey: ['backups', 'platform'], queryFn: () => api.get<Backup[]>('/platform/backups') });
  const create = useMutation({
    mutationFn: () => api.post('/platform/backups'),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['backups', 'platform'] }),
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Stack gap="lg">
      <Card padding={0}>
        <Group justify="space-between" className="panel-header" wrap="nowrap">
          <div>
            <Text fw={600}>Platform backups</Text>
            <Text size="sm" c="dimmed">
              Database and encryption key. To restore, stop Moonshed and extract the archive into <Code>runtime/data/</Code>.
            </Text>
          </div>
          <Button variant="default" onClick={() => create.mutate()} loading={create.isPending}>
            Back up now
          </Button>
        </Group>
        <BackupTable backups={backups.data ?? []} />
      </Card>
      {settings.data && <PlatformSchedule key={JSON.stringify(settings.data.settings)} stored={settings.data.settings} />}
    </Stack>
  );
}

function PlatformSchedule({ stored }: { stored: Settings }) {
  const [enabled, setEnabled] = useState(stored.backup_enabled);
  const [cron, setCron] = useState(stored.backup_cron);
  const [keep, setKeep] = useState(stored.backup_keep);
  const save = useMutation({
    mutationFn: () => api.put<SettingsResponse>('/settings', { ...stored, backup_enabled: enabled, backup_cron: cron, backup_keep: keep }),
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      notifications.show({ color: 'green', message: 'Schedule saved' });
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const changed = enabled !== stored.backup_enabled || cron !== stored.backup_cron || keep !== stored.backup_keep;

  return (
    <Card padding={0}>
      <Group justify="space-between" className="panel-header">
        <Text fw={600}>Platform backup schedule</Text>
        {changed && (
          <Button size="sm" onClick={() => save.mutate()} loading={save.isPending} disabled={enabled && !cron.trim()}>
            Save
          </Button>
        )}
      </Group>
      <Stack p="lg">
        <Switch label="Back up on a schedule" checked={enabled} onChange={(e) => setEnabled(e.currentTarget.checked)} />
        {enabled && (
          <Group align="flex-start">
            <TextInput
              label="Cron expression"
              description="Server time, e.g. 0 2 * * * for 2:00 every day"
              placeholder="0 2 * * *"
              required
              value={cron}
              onChange={(e) => setCron(e.currentTarget.value)}
              error={!cron.trim() ? 'Required' : undefined}
              w={300}
            />
            <NumberInput label="Keep" description="Newest backups" min={1} value={keep} onChange={(v) => setKeep(Number(v) || 1)} w={140} />
          </Group>
        )}
      </Stack>
    </Card>
  );
}
