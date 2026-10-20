import { useState } from 'react';
import { Alert, Anchor, Button, Checkbox, Code, CopyButton, Group, Modal, ScrollArea, SimpleGrid, Stack, Switch, Text, Textarea, TextInput } from '@mantine/core';
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
import type { ProxyCheck, Settings, SettingsResponse } from '../../types';
import { Section } from '../app/Settings';

const privateRanges = 'private_ranges';

export function ProxySettingsPage() {
  const query = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  if (!query.data) return null;
  const data = query.data;
  const s = data.settings;
  // Each section is keyed by its own stored fields, so saving one section never resets unsaved edits in another.
  const key = (...fields: (keyof Settings)[]) => JSON.stringify(fields.map((f) => s[f]));

  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Reverse Proxy (Caddy)" />
      <ProxySection key={key('proxy_enabled', 'acme_email')} data={data} />
      <PanelSection key={key('panel_domain', 'panel_tls')} data={data} />
      <TrustedSection key={key('trusted_proxies')} stored={s} />
      <AdvancedSection key={key('global_options', 'extra_sites')} stored={s} />
    </Stack>
  );
}

// useSaveSettings saves some settings fields on top of whatever is stored now.
function useSaveSettings(message: string) {
  return useMutation({
    mutationFn: (patch: Partial<Settings>) => {
      const stored = queryClient.getQueryData<SettingsResponse>(['settings'])!.settings;
      return api.put<SettingsResponse>('/settings', { ...stored, ...patch });
    },
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      queryClient.invalidateQueries({ queryKey: ['server'] });
      if (res.error) {
        notifications.show({ color: 'red', title: 'Proxy could not start', message: res.error });
      } else {
        notifications.show({ color: 'green', message });
      }
    },
    onError: (err) => notifications.show({ color: 'red', title: 'Not saved', message: err.message }),
  });
}

function ProxySection({ data }: { data: SettingsResponse }) {
  const { proxy, settings } = data;
  const [enabled, setEnabled] = useState(settings.proxy_enabled);
  const [email, setEmail] = useState(settings.acme_email);
  const [viewing, setViewing] = useState(false);
  const save = useSaveSettings('Proxy saved');
  const changed = enabled !== settings.proxy_enabled || email !== settings.acme_email;

  return (
    <Section
      title="Reverse proxy"
      description={
        <>
          Routes your domains to your apps with HTTPS. Needs ports {proxy.http_addr.replace(/^.*:/, '')} and {proxy.https_addr.replace(/^.*:/, '')} open.
        </>
      }
      onSave={changed ? () => save.mutate({ proxy_enabled: enabled, acme_email: email }) : undefined}
      saving={save.isPending}
    >
      {proxy.error && (
        <Alert color="red" variant="light">
          {proxy.error}
        </Alert>
      )}
      <Group justify="space-between">
        <Switch label={proxy.listening ? 'Enabled (listening)' : 'Enabled'} checked={enabled} onChange={(e) => setEnabled(e.currentTarget.checked)} />
        <Button variant="default" size="xs" onClick={() => setViewing(true)}>
          View Caddyfile
        </Button>
      </Group>
      <TextInput
        label="Let's Encrypt email (optional)"
        placeholder="you@example.com"
        value={email}
        onChange={(e) => setEmail(e.currentTarget.value)}
        maw={420}
      />
      <CaddyfileModal opened={viewing} onClose={() => setViewing(false)} />
    </Section>
  );
}

// loopbackOnly says panel_addr binds to this machine only; ":5050" or "0.0.0.0:5050" listen on every interface.
function loopbackOnly(addr: string): boolean {
  const host = addr.slice(0, addr.lastIndexOf(':'));
  return ['127.0.0.1', 'localhost', '::1', '[::1]'].includes(host);
}

function PanelSection({ data }: { data: SettingsResponse }) {
  const { settings, panel_certificate } = data;
  const [domain, setDomain] = useState(settings.panel_domain);
  const [tls, setTLS] = useState(settings.panel_tls);
  const [cert, setCert] = useState<CertificateValue>(emptyCertificate);
  const save = useSaveSettings('Panel access saved');
  const local = loopbackOnly(data.panel_addr);
  const customCert = !!domain && tls === 'custom';
  const savedCustomCert = settings.panel_domain === domain && panel_certificate;
  const certChanged = customCert && certificateReady(cert);
  const changed = domain !== settings.panel_domain || tls !== settings.panel_tls || certChanged;

  const submit = async () => {
    if (certChanged) {
      const upload = new FormData();
      upload.append('host', domain);
      appendCertificate(upload, cert);
      try {
        await api.post('/settings/panel-certificate', upload);
      } catch (err) {
        notifications.show({ color: 'red', title: 'Certificate not saved', message: (err as Error).message });
        return;
      }
    }
    save.mutate({ panel_domain: domain, panel_tls: tls }, { onSuccess: () => setCert(emptyCertificate) });
  };

  return (
    <Section
      title="Panel access"
      description={
        <>
          Serve the panel on its own domain with HTTPS so it can be reached securely from the web. GitHub App and push webhooks need this.
          <br />
          {local ? (
            <>
              Without a domain, it's only reachable at <Code>{data.panel_addr}</Code> from this machine.
            </>
          ) : (
            <>
              Otherwise, the panel is always reachable at <Code>{data.panel_addr}</Code>. Block that port in your firewall if it shouldn't be accessed to the internet.
            </>
          )}
        </>
      }
      onSave={changed ? submit : undefined}
      saving={save.isPending}
    >
      <SimpleGrid cols={{ base: 1, sm: 2 }}>
        <TextInput label="Panel domain (optional)" placeholder="panel.example.com" value={domain} onChange={(e) => setDomain(e.currentTarget.value)} />
        <TLSPicker value={tls} onChange={setTLS} disabled={!domain} />
      </SimpleGrid>
      {domain && !settings.proxy_enabled && <ProxyOff>The panel domain needs the reverse proxy.</ProxyOff>}
      {customCert && (
        <Stack gap="xs">
          <Text size="sm" fw={500}>
            Certificate for {domain}
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
    </Section>
  );
}

function TrustedSection({ stored }: { stored: Settings }) {
  const [trusted, setTrustedList] = useState(stored.trusted_proxies ?? []);
  const save = useSaveSettings('Trusted proxies saved');
  const usePrivate = trusted.includes(privateRanges);
  const ranges = trusted.filter((p) => p !== privateRanges);
  const setTrusted = (priv: boolean, list: string[]) => setTrustedList([...(priv ? [privateRanges] : []), ...list]);
  const changed = JSON.stringify(trusted.filter(Boolean)) !== JSON.stringify(stored.trusted_proxies ?? []);

  return (
    <Section
      title="Trusted proxies"
      description={
        <>
          If Cloudflare or a load balancer sits in front of this server, add it here so logs show the real visitor IP from <Code>X-Forwarded-For</Code>.
        </>
      }
      onSave={changed ? () => save.mutate({ trusted_proxies: trusted.filter(Boolean) }) : undefined}
      saving={save.isPending}
    >
      <Checkbox
        label="Private networks (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, localhost)"
        checked={usePrivate}
        onChange={(e) => setTrusted(e.currentTarget.checked, ranges)}
      />
      <Textarea
        label="Other ranges"
        description={
          <>
            One IP or CIDR range per line. Cloudflare publishes theirs at{' '}
            <Anchor href="https://www.cloudflare.com/ips/" target="_blank" size="xs">
              cloudflare.com/ips
            </Anchor>
            .
          </>
        }
        autosize
        minRows={3}
        ff="monospace"
        value={ranges.join('\n')}
        onChange={(e) =>
          setTrusted(
            usePrivate,
            e.currentTarget.value.split(/\n/).map((s) => s.trim()),
          )
        }
        onBlur={() => setTrusted(usePrivate, ranges.filter(Boolean))}
      />
    </Section>
  );
}

function AdvancedSection({ stored }: { stored: Settings }) {
  const [globalOptions, setGlobalOptions] = useState(stored.global_options);
  const [extraSites, setExtraSites] = useState(stored.extra_sites);
  const [check, setCheck] = useState<ProxyCheck | null>(null);
  const save = useSaveSettings('Advanced settings saved');
  const fields = { global_options: globalOptions, extra_sites: extraSites };
  const validate = useMutation({
    mutationFn: () => api.post<ProxyCheck>('/settings/proxy-check', { ...queryClient.getQueryData<SettingsResponse>(['settings'])!.settings, ...fields }),
    onSuccess: setCheck,
    onError: (err) => setCheck({ ok: false, error: err.message, caddyfile: '' }),
  });
  const changed = globalOptions !== stored.global_options || extraSites !== stored.extra_sites;

  return (
    <Section
      title="Advanced"
      description="Raw Caddyfile for anything the UI doesn't cover. If it ever breaks the config, the proxy runs without it and shows the error."
      onSave={changed ? () => save.mutate(fields) : undefined}
      saving={save.isPending}
    >
      <Textarea
        label="Global options"
        description={
          <>
            Lines inside Caddy's global options block, e.g. <Code>servers {'{ timeouts { idle 5m } }'}</Code>.{' '}
            <Anchor href="https://caddyserver.com/docs/caddyfile/options" target="_blank" size="xs">
              Options reference
            </Anchor>
          </>
        }
        autosize
        minRows={4}
        ff="monospace"
        styles={{ input: { fontSize: 13 } }}
        value={globalOptions}
        onChange={(e) => {
          setGlobalOptions(e.currentTarget.value);
          setCheck(null);
        }}
      />
      <Textarea
        label="Extra sites"
        description="Site blocks appended to the Caddyfile."
        placeholder={'old.example.com {\n\tredir https://example.com{uri} permanent\n}'}
        autosize
        minRows={6}
        ff="monospace"
        styles={{ input: { fontSize: 13 } }}
        value={extraSites}
        onChange={(e) => {
          setExtraSites(e.currentTarget.value);
          setCheck(null);
        }}
      />
      <Group>
        <Button variant="default" onClick={() => validate.mutate()} loading={validate.isPending}>
          Validate
        </Button>
        {check && (
          <Text size="sm" c={check.ok ? 'green' : 'red'}>
            {check.ok ? 'The config is valid.' : 'The config has an error.'}
          </Text>
        )}
      </Group>
      {check?.error && (
        <Alert color="red" variant="light">
          <Text size="sm" ff="monospace" style={{ whiteSpace: 'pre-wrap' }}>
            {check.error}
          </Text>
        </Alert>
      )}
    </Section>
  );
}

function CaddyfileModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const query = useQuery({
    queryKey: ['proxy', 'caddyfile'],
    queryFn: () => api.get<{ caddyfile: string }>('/proxy/caddyfile'),
    enabled: opened,
    staleTime: 0,
  });
  const text = query.data?.caddyfile ?? '';
  return (
    <Modal opened={opened} onClose={onClose} size="xl" title="Generated Caddyfile">
      <Stack>
        <Text size="sm" c="dimmed">
          Built from your domains and the settings on this page.
        </Text>
        <ScrollArea.Autosize mah="65vh">
          <Code block>{query.isLoading ? 'Loading…' : text}</Code>
        </ScrollArea.Autosize>
        <Group justify="flex-end">
          <CopyButton value={text}>
            {({ copied, copy }) => (
              <Button variant="default" onClick={copy} disabled={!text}>
                {copied ? 'Copied' : 'Copy'}
              </Button>
            )}
          </CopyButton>
        </Group>
      </Stack>
    </Modal>
  );
}
