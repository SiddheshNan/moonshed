import { useEffect, useState } from 'react';
import {
  ActionIcon,
  Alert,
  Button,
  Code,
  Drawer,
  Group,
  PasswordInput,
  ScrollArea,
  Select,
  Stack,
  Switch,
  Tabs,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core';
import { notifications } from '@mantine/notifications';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { useMutation } from '@tanstack/react-query';

import { api } from '../../api';
import { appendCertificate, CertificateFields, CertificateSummary, certificateReady, emptyCertificate, type CertificateValue } from '../../components/Certificate';
import { TLSPicker } from '../../components/TLSPicker';
import type { Domain, DomainOptions, HeaderRule, ProxyCheck, TLSMode } from '../../types';

interface AuthUser {
  user: string;
  password: string;
  saved: boolean; // has a stored password, so leaving it empty keeps it
}

interface Form {
  host: string;
  path: string;
  tls: TLSMode;
  options: Omit<DomainOptions, 'basic_auth' | 'allow_ips'>;
  users: AuthUser[];
  allowIPs: string;
  advanced: string;
}

function toForm(d: Domain): Form {
  const o = d.options;
  return {
    host: d.host,
    path: d.path,
    tls: d.tls,
    options: {
      force_https: o.force_https,
      hsts: o.hsts,
      redirect_www: o.redirect_www,
      compress: o.compress,
      strip_prefix: o.strip_prefix,
      request_headers: o.request_headers ?? [],
      response_headers: o.response_headers ?? [],
      max_body: o.max_body,
    },
    users: (o.basic_auth ?? []).map((u) => ({ user: u.user, password: '', saved: true })),
    allowIPs: (o.allow_ips ?? []).join('\n'),
    advanced: d.advanced,
  };
}

function toPayload(f: Form) {
  return {
    host: f.host,
    path: f.path,
    tls: f.tls,
    advanced: f.advanced,
    options: {
      ...f.options,
      basic_auth: f.users.map(({ user, password }) => ({ user, password })),
      allow_ips: f.allowIPs
        .split(/[\s,]+/)
        .map((s) => s.trim())
        .filter(Boolean),
    },
  };
}

// siteBlock picks the block for host out of a generated Caddyfile.
function siteBlock(caddyfile: string, host: string): string {
  const lines = caddyfile.split('\n');
  const start = lines.findIndex((l) => /^\S.*\{$/.test(l) && l.split(/[\s,]+/).some((a) => a === host || a === `http://${host}`));
  if (start < 0) return caddyfile;
  const end = lines.findIndex((l, i) => i > start && l === '}');
  return lines.slice(start, end < 0 ? undefined : end + 1).join('\n');
}

export function DomainEditor({ domain, onClose, onSaved }: { domain: Domain | null; onClose: () => void; onSaved: () => void }) {
  const [form, setForm] = useState<Form | null>(null);
  const [cert, setCert] = useState<CertificateValue>(emptyCertificate);
  const [check, setCheck] = useState<ProxyCheck | null>(null);
  const [tab, setTab] = useState<string | null>('general');

  useEffect(() => {
    setForm(domain ? toForm(domain) : null);
    setCert(emptyCertificate);
    setCheck(null);
    setTab('general');
  }, [domain]);

  const needsCert = !!form && !!domain && form.tls === 'custom' && (form.host !== domain.host || !domain.certificate);

  const save = useMutation({
    mutationFn: () => {
      const payload = toPayload(form!);
      if (form!.tls === 'custom' && certificateReady(cert)) {
        const body = new FormData();
        body.append('domain', JSON.stringify(payload));
        appendCertificate(body, cert);
        return api.put<Domain>(`/domains/${domain!.id}`, body);
      }
      return api.put<Domain>(`/domains/${domain!.id}`, payload);
    },
    onSuccess: () => {
      notifications.show({ color: 'green', message: `${form!.host}${form!.path === '/' ? '' : form!.path} saved` });
      onSaved();
      onClose();
    },
    onError: (err) => notifications.show({ color: 'red', title: 'Not saved', message: err.message }),
  });
  const validate = useMutation({
    mutationFn: () => api.post<ProxyCheck>(`/domains/${domain!.id}/check`, toPayload(form!)),
    onSuccess: setCheck,
    onError: (err) => setCheck({ ok: false, error: err.message, caddyfile: '' }),
  });

  if (!form || !domain) {
    return <Drawer opened={false} onClose={onClose} />;
  }
  const set = (patch: Partial<Form>) => {
    setForm({ ...form, ...patch });
    setCheck(null);
  };
  const setOpt = (patch: Partial<Form['options']>) => set({ options: { ...form.options, ...patch } });
  const https = form.tls !== 'none';

  return (
    <Drawer opened onClose={onClose} position="right" size="xl" title={<Text fw={600}>{domain.host + (domain.path === '/' ? '' : domain.path)}</Text>}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate();
        }}
      >
        <Stack>
          <Tabs value={tab} onChange={setTab}>
            <Tabs.List mb="md">
              <Tabs.Tab value="general">General</Tabs.Tab>
              <Tabs.Tab value="access">Access</Tabs.Tab>
              <Tabs.Tab value="headers">Headers</Tabs.Tab>
              <Tabs.Tab value="advanced">Advanced</Tabs.Tab>
            </Tabs.List>

            <Tabs.Panel value="general">
              <Stack>
                <Group align="flex-end" wrap="wrap">
                  <TextInput label="Host" value={form.host} onChange={(e) => set({ host: e.currentTarget.value })} required style={{ flex: 2, minWidth: 220 }} />
                  <TextInput label="Path" value={form.path} onChange={(e) => set({ path: e.currentTarget.value })} w={140} />
                </Group>
                <TLSPicker value={form.tls} onChange={(tls) => set({ tls })} />
                {form.tls === 'custom' && (
                  <Stack gap={4}>
                    {domain.certificate && form.host === domain.host && <CertificateSummary cert={domain.certificate} />}
                    {(needsCert || certificateReady(cert)) && (
                      <Text size="sm" fw={500}>
                        Certificate for {form.host}
                      </Text>
                    )}
                    {needsCert ? (
                      <CertificateFields value={cert} onChange={setCert} />
                    ) : (
                      <Text size="xs" c="dimmed">
                        Use the certificate button in the domain list to replace it.
                      </Text>
                    )}
                  </Stack>
                )}

                <Text size="sm" fw={500} mt="xs">
                  Host-wide (every path on {form.host || 'this host'})
                </Text>
                <Switch
                  label="Redirect HTTP to HTTPS"
                  disabled={!https}
                  checked={https && form.options.force_https}
                  onChange={(e) => setOpt({ force_https: e.currentTarget.checked })}
                />
                <Switch
                  label="HSTS"
                  description="Tell browsers to only use HTTPS for a year. Hard to undo."
                  disabled={!https}
                  checked={https && form.options.hsts}
                  onChange={(e) => setOpt({ hsts: e.currentTarget.checked })}
                />
                <Switch
                  label={`Redirect www.${form.host || 'host'} here`}
                  description="Its DNS must point at this server too."
                  checked={form.options.redirect_www}
                  onChange={(e) => setOpt({ redirect_www: e.currentTarget.checked })}
                />

                <Text size="sm" fw={500} mt="xs">
                  This path
                </Text>
                <Switch
                  label="Compress responses"
                  description="gzip and zstd"
                  checked={form.options.compress}
                  onChange={(e) => setOpt({ compress: e.currentTarget.checked })}
                />
                <Switch
                  label={`Strip ${form.path === '/' ? 'the path' : form.path} before passing requests on`}
                  description="The app sees /users instead of /api/users"
                  disabled={form.path === '/' || form.path === ''}
                  checked={form.options.strip_prefix && form.path !== '/'}
                  onChange={(e) => setOpt({ strip_prefix: e.currentTarget.checked })}
                />
                <TextInput
                  label="Max request body"
                  description="Empty = no limit"
                  placeholder="10MB"
                  value={form.options.max_body}
                  onChange={(e) => setOpt({ max_body: e.currentTarget.value })}
                  w={200}
                />
              </Stack>
            </Tabs.Panel>

            <Tabs.Panel value="access">
              <Stack>
                <div>
                  <Text size="sm" fw={500}>
                    Password protection
                  </Text>
                  <Text size="xs" c="dimmed">
                    Visitors need one of these logins (HTTP basic auth). Leave a password empty to keep the current one.
                  </Text>
                </div>
                {form.users.map((u, i) => (
                  <Group key={i} align="flex-end" wrap="nowrap">
                    <TextInput
                      label={i === 0 ? 'Username' : undefined}
                      value={u.user}
                      onChange={(e) => set({ users: form.users.map((x, j) => (j === i ? { ...x, user: e.currentTarget.value, saved: false } : x)) })}
                      style={{ flex: 1 }}
                    />
                    <PasswordInput
                      label={i === 0 ? 'Password' : undefined}
                      placeholder={u.saved ? 'unchanged' : ''}
                      value={u.password}
                      onChange={(e) => set({ users: form.users.map((x, j) => (j === i ? { ...x, password: e.currentTarget.value } : x)) })}
                      style={{ flex: 1 }}
                    />
                    <ActionIcon variant="subtle" color="red" mb={4} onClick={() => set({ users: form.users.filter((_, j) => j !== i) })}>
                      <IconTrash size={16} />
                    </ActionIcon>
                  </Group>
                ))}
                <Button
                  variant="default"
                  size="xs"
                  w="fit-content"
                  leftSection={<IconPlus size={14} />}
                  onClick={() => set({ users: [...form.users, { user: '', password: '', saved: false }] })}
                >
                  Add login
                </Button>

                <Textarea
                  mt="md"
                  label="Allowed IP addresses"
                  description="One IP or CIDR range per line. Everyone else gets 403. Empty = everyone."
                  placeholder={'203.0.113.7\n10.0.0.0/8'}
                  autosize
                  minRows={3}
                  ff="monospace"
                  value={form.allowIPs}
                  onChange={(e) => set({ allowIPs: e.currentTarget.value })}
                />
                <Text size="xs" c="dimmed">
                  Behind Cloudflare or a load balancer, add it under Settings › Proxy › Trusted proxies so the real visitor IP is used.
                </Text>
              </Stack>
            </Tabs.Panel>

            <Tabs.Panel value="headers">
              <Stack gap="xl">
                <HeaderRules
                  title="Request headers"
                  description="Changed before the request reaches the app."
                  rules={form.options.request_headers ?? []}
                  onChange={(request_headers) => setOpt({ request_headers })}
                />
                <HeaderRules
                  title="Response headers"
                  description="Changed before the response reaches the visitor."
                  rules={form.options.response_headers ?? []}
                  onChange={(response_headers) => setOpt({ response_headers })}
                />
              </Stack>
            </Tabs.Panel>

            <Tabs.Panel value="advanced">
              <Stack>
                <Text size="sm" c="dimmed">
                  Caddyfile directives for this path, added before the request is passed to the app. Anything from the{' '}
                  <a href="https://caddyserver.com/docs/caddyfile/directives" target="_blank" rel="noreferrer">
                    Caddy docs
                  </a>{' '}
                  works here, for example <Code>rewrite</Code>, <Code>redir</Code>, <Code>header</Code> or <Code>respond</Code>.
                  Saving checks the whole config first.
                </Text>
                <Textarea
                  autosize
                  minRows={8}
                  ff="monospace"
                  placeholder={'header X-Robots-Tag "noindex"\nredir /old /new permanent'}
                  value={form.advanced}
                  onChange={(e) => set({ advanced: e.currentTarget.value })}
                  styles={{ input: { fontSize: 13 } }}
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
                {check?.caddyfile && (
                  <Stack gap={4}>
                    <Text size="sm" fw={500}>
                      Generated config for {form.host}
                    </Text>
                    <ScrollArea.Autosize mah={360}>
                      <Code block>{siteBlock(check.caddyfile, form.host)}</Code>
                    </ScrollArea.Autosize>
                  </Stack>
                )}
              </Stack>
            </Tabs.Panel>
          </Tabs>

          <Group justify="flex-end" mt="md">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={save.isPending} disabled={needsCert && !certificateReady(cert)}>
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </Drawer>
  );
}

function HeaderRules({ title, description, rules, onChange }: { title: string; description: string; rules: HeaderRule[]; onChange: (r: HeaderRule[]) => void }) {
  const update = (i: number, patch: Partial<HeaderRule>) => onChange(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return (
    <Stack gap="xs">
      <div>
        <Text size="sm" fw={500}>
          {title}
        </Text>
        <Text size="xs" c="dimmed">
          {description}
        </Text>
      </div>
      {rules.map((r, i) => (
        <Group key={i} wrap="nowrap" align="flex-start">
          <Select
            w={110}
            value={r.op}
            onChange={(op) => update(i, { op: (op ?? 'set') as HeaderRule['op'] })}
            data={[
              { value: 'set', label: 'Set' },
              { value: 'add', label: 'Add' },
              { value: 'remove', label: 'Remove' },
            ]}
            allowDeselect={false}
          />
          <TextInput placeholder="Header-Name" value={r.name} onChange={(e) => update(i, { name: e.currentTarget.value })} style={{ flex: 1 }} />
          <TextInput
            placeholder={r.op === 'remove' ? '' : 'value'}
            disabled={r.op === 'remove'}
            value={r.op === 'remove' ? '' : r.value}
            onChange={(e) => update(i, { value: e.currentTarget.value })}
            style={{ flex: 1 }}
          />
          <ActionIcon variant="subtle" color="red" mt={6} onClick={() => onChange(rules.filter((_, j) => j !== i))}>
            <IconTrash size={16} />
          </ActionIcon>
        </Group>
      ))}
      <Button variant="default" size="xs" w="fit-content" leftSection={<IconPlus size={14} />} onClick={() => onChange([...rules, { op: 'set', name: '', value: '' }])}>
        Add header
      </Button>
    </Stack>
  );
}
