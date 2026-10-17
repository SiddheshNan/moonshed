import { useEffect, useState } from 'react';
import { Alert, Anchor, Button, Card, Checkbox, Code, CopyButton, Group, Modal, ScrollArea, Stack, Switch, Text, Textarea, TextInput } from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { useMutation, useQuery } from '@tanstack/react-query';

import { api, queryClient } from '../../api';
import { PageHeader } from '../../components/PageHeader';
import type { ProxyCheck, Settings, SettingsResponse } from '../../types';

const privateRanges = 'private_ranges';

export function ProxySettingsPage() {
  const query = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });
  const [form, setForm] = useState<Settings | null>(null);
  const [check, setCheck] = useState<ProxyCheck | null>(null);
  const [viewing, setViewing] = useState(false);
  useEffect(() => {
    if (query.data && !form) setForm(query.data.settings);
  }, [query.data, form]);

  // Only the proxy fields come from this form; everything else stays as stored.
  const merged = (f: Settings): Settings => {
    const stored = queryClient.getQueryData<SettingsResponse>(['settings'])?.settings ?? f;
    return {
      ...stored,
      proxy_enabled: f.proxy_enabled,
      acme_email: f.acme_email,
      trusted_proxies: f.trusted_proxies,
      global_options: f.global_options,
      extra_sites: f.extra_sites,
    };
  };
  const save = useMutation({
    mutationFn: (f: Settings) => api.put<SettingsResponse>('/settings', merged(f)),
    onSuccess: (res) => {
      queryClient.setQueryData(['settings'], res);
      queryClient.invalidateQueries({ queryKey: ['server'] });
      setForm(res.settings);
      if (res.error) {
        notifications.show({ color: 'red', title: 'Proxy could not start', message: res.error });
      } else {
        notifications.show({ color: 'green', message: 'Proxy settings saved' });
      }
    },
    onError: (err) => notifications.show({ color: 'red', title: 'Not saved', message: err.message }),
  });
  const validate = useMutation({
    mutationFn: (f: Settings) => api.post<ProxyCheck>('/settings/proxy-check', merged(f)),
    onSuccess: setCheck,
    onError: (err) => setCheck({ ok: false, error: err.message, caddyfile: '' }),
  });

  if (!form || !query.data) return null;
  const { proxy } = query.data;
  const set = (patch: Partial<Settings>) => {
    setForm({ ...form, ...patch });
    setCheck(null);
  };
  const trusted = form.trusted_proxies ?? [];
  const usePrivate = trusted.includes(privateRanges);
  const ranges = trusted.filter((p) => p !== privateRanges);
  const setTrusted = (priv: boolean, list: string[]) => set({ trusted_proxies: [...(priv ? [privateRanges] : []), ...list] });

  return (
    <Stack gap="lg" maw={960}>
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Proxy" />
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(form);
        }}
      >
        <Stack gap="lg">
          <Card>
            <Stack>
              <Group justify="space-between" align="flex-start">
                <div>
                  <Text fw={600}>Reverse proxy</Text>
                  <Text size="sm" c="dimmed">
                    Listens on {proxy.http_addr} and {proxy.https_addr}, set in <Code>{query.data.config_file}</Code>.
                  </Text>
                </div>
                <Button variant="default" onClick={() => setViewing(true)}>
                  View Caddyfile
                </Button>
              </Group>
              {proxy.error && (
                <Alert color="red" variant="light">
                  {proxy.error}
                </Alert>
              )}
              <Switch label={proxy.listening ? 'Enabled (listening)' : 'Enabled'} checked={form.proxy_enabled} onChange={(e) => set({ proxy_enabled: e.currentTarget.checked })} />
              <TextInput
                label="Let's Encrypt email (optional)"
                placeholder="you@example.com"
                value={form.acme_email}
                onChange={(e) => set({ acme_email: e.currentTarget.value })}
                maw={420}
              />
            </Stack>
          </Card>

          <Card>
            <Stack>
              <div>
                <Text fw={600}>Trusted proxies</Text>
                <Text size="sm" c="dimmed">
                  Proxies in front of this server, like Cloudflare or a load balancer. Their <Code>X-Forwarded-For</Code> is trusted for the visitor's IP. Only
                  list ones you control.
                </Text>
              </div>
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
            </Stack>
          </Card>

          <Card>
            <Stack>
              <div>
                <Text fw={600}>Advanced</Text>
                <Text size="sm" c="dimmed">
                  Raw Caddyfile for anything the UI doesn't cover. If it ever breaks the config, the proxy runs without it and shows the error.
                </Text>
              </div>
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
                value={form.global_options}
                onChange={(e) => set({ global_options: e.currentTarget.value })}
              />
              <Textarea
                label="Extra sites"
                description="Site blocks appended to the Caddyfile."
                placeholder={'old.example.com {\n\tredir https://example.com{uri} permanent\n}'}
                autosize
                minRows={6}
                ff="monospace"
                styles={{ input: { fontSize: 13 } }}
                value={form.extra_sites}
                onChange={(e) => set({ extra_sites: e.currentTarget.value })}
              />
              <Group>
                <Button variant="default" onClick={() => validate.mutate(form)} loading={validate.isPending}>
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
            </Stack>
          </Card>

          <Group justify="flex-end">
            <Button type="submit" loading={save.isPending}>
              Save proxy settings
            </Button>
          </Group>
        </Stack>
      </form>
      <CaddyfileModal opened={viewing} onClose={() => setViewing(false)} />
    </Stack>
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
          Built from your domains and the settings on this page, as saved. Read-only: change it through the UI or the Advanced boxes.
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
