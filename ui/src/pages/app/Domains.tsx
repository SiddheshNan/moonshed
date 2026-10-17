import { useState } from 'react';
import { ActionIcon, Anchor, Badge, Button, Card, Code, Fieldset, Group, Modal, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core';
import { modals } from '@mantine/modals';
import { notifications } from '@mantine/notifications';
import { IconCertificate, IconPencil, IconPlus, IconTrash } from '@tabler/icons-react';
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
import { tlsLabels, TLSPicker } from '../../components/TLSPicker';
import type { App, Domain, ServerInfo, TLSMode } from '../../types';
import { DomainEditor } from './DomainEditor';


export function Domains({ app }: { app: App }) {
  const [host, setHost] = useState('');
  const [path, setPath] = useState('/');
  const [tls, setTLS] = useState<TLSMode>('letsencrypt');
  const [cert, setCert] = useState<CertificateValue>(emptyCertificate);
  const [replacing, setReplacing] = useState<Domain | null>(null);
  const [editing, setEditing] = useState<Domain | null>(null);
  const server = useQuery({ queryKey: ['server'], queryFn: () => api.get<ServerInfo>('/server') });

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['app', app.id] });
    queryClient.invalidateQueries({ queryKey: ['apps'] });
  };
  const add = useMutation({
    mutationFn: () => {
      if (tls !== 'custom') {
        return api.post<Domain>(`/apps/${app.id}/domains`, { host, path, tls });
      }
      const form = new FormData();
      form.append('host', host);
      form.append('path', path);
      form.append('tls', tls);
      appendCertificate(form, cert);
      return api.post<Domain>(`/apps/${app.id}/domains`, form);
    },
    onSuccess: () => {
      setHost('');
      setPath('/');
      setCert(emptyCertificate);
      refresh();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.delete(`/domains/${id}`),
    onSuccess: refresh,
  });

  const proxyOff = server.data && !server.data.proxy.enabled;
  const blocked = proxyOff || !app.port;

  // A certificate belongs to a host, so domains that only differ by path share it.
  const sharedPaths = (d: Domain) => app.domains.filter((o) => o.host === d.host && o.tls === 'custom').length > 1;

  return (
    <Stack gap="lg">
      <Card padding={0}>
        <Text fw={600} p="md">
          Domains
        </Text>
        <Table.ScrollContainer minWidth={620}>
          <Table verticalSpacing="sm">
            <Table.Tbody>
              {app.domains.map((d) => (
                <Table.Tr key={d.id}>
                  <Table.Td>
                    <Anchor href={`${d.tls === 'none' ? 'http' : 'https'}://${d.host}${d.path}`} target="_blank" size="sm">
                      {d.host}
                      {d.path === '/' ? '' : d.path}
                    </Anchor>
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Badge variant="light" color={d.tls === 'none' ? 'gray' : 'green'}>
                        {tlsLabels[d.tls]}
                      </Badge>
                      {optionBadges(d).map((label) => (
                        <Badge key={label} variant="outline" color="gray">
                          {label}
                        </Badge>
                      ))}
                    </Group>
                  </Table.Td>
                  <Table.Td maw={320}>
                    {d.tls === 'custom' ? (
                      <CertificateSummary cert={d.certificate} />
                    ) : (
                      <Text size="sm" c="dimmed">
                        → 127.0.0.1:{app.port}
                      </Text>
                    )}
                  </Table.Td>
                  <Table.Td>
                    <Group gap={4} justify="flex-end" wrap="nowrap">
                      <Tooltip label="Edit proxy settings">
                        <ActionIcon variant="subtle" color="gray" onClick={() => setEditing(d)}>
                          <IconPencil size={16} />
                        </ActionIcon>
                      </Tooltip>
                      {d.tls === 'custom' && (
                        <Tooltip label="Replace certificate">
                          <ActionIcon variant="subtle" color="gray" onClick={() => setReplacing(d)}>
                            <IconCertificate size={16} />
                          </ActionIcon>
                        </Tooltip>
                      )}
                      <ActionIcon
                        variant="subtle"
                        color="red"
                        onClick={() =>
                          modals.openConfirmModal({
                            title: `Remove ${d.host}${d.path === '/' ? '' : d.path}?`,
                            labels: { confirm: 'Remove', cancel: 'Cancel' },
                            confirmProps: { color: 'red' },
                            onConfirm: () => remove.mutate(d.id),
                          })
                        }
                      >
                        <IconTrash size={16} />
                      </ActionIcon>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
        {app.domains.length === 0 && (
          <Text c="dimmed" size="sm" px="md" pb="md">
            No domains yet.
          </Text>
        )}
      </Card>

      <Card>
        <Text fw={600}>Add domain</Text>
        {blocked && (
          <Text size="sm" c="dimmed">
            {proxyOff ? (
              <>
                The reverse proxy is off.{' '}
                <Anchor component={Link} to="/settings/proxy" size="sm">
                  Turn it on
                </Anchor>{' '}
                to serve domains.
              </>
            ) : (
              <>
                Set the app's port first, on the{' '}
                <Anchor component={Link} to={`/apps/${app.id}`} size="sm">
                  Overview
                </Anchor>{' '}
                or with <Code>port:</Code> in app.yaml.
              </>
            )}
          </Text>
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            add.mutate();
          }}
        >
          <Fieldset variant="unstyled" disabled={blocked} mt="md">
            <Stack>
              <Group align="flex-end" wrap="wrap">
                <TextInput label="Host" placeholder="app.example.com" value={host} onChange={(e) => setHost(e.currentTarget.value)} required style={{ flex: 2, minWidth: 220 }} />
                <TextInput label="Path" value={path} onChange={(e) => setPath(e.currentTarget.value)} w={120} />
              </Group>
              <TLSPicker value={tls} onChange={setTLS} />
              {tls === 'custom' && (
                <Stack gap={4}>
                  <Text size="sm" fw={500}>
                    Certificate for {host || 'this domain'}
                  </Text>
                  <CertificateFields value={cert} onChange={setCert} />
                </Stack>
              )}
              <Group justify="space-between" align="flex-start">
                <Text size="xs" c="dimmed" maw={520}>
                  Point the domain's DNS at this server first.
                </Text>
                <Button
                  type="submit"
                  leftSection={<IconPlus size={16} />}
                  loading={add.isPending}
                  disabled={blocked || (tls === 'custom' && !certificateReady(cert))}
                >
                  Add domain
                </Button>
              </Group>
            </Stack>
          </Fieldset>
        </form>
      </Card>

      <DomainEditor domain={editing} onClose={() => setEditing(null)} onSaved={refresh} />
      <ReplaceCertificate domain={replacing} shared={replacing ? sharedPaths(replacing) : false} onClose={() => setReplacing(null)} onDone={refresh} />
    </Stack>
  );
}

// optionBadges names the non-default settings on a domain.
function optionBadges(d: Domain): string[] {
  const o = d.options;
  const out: string[] = [];
  if (o.basic_auth?.length) out.push('Password');
  if (o.allow_ips?.length) out.push('IP allow list');
  if (o.request_headers?.length || o.response_headers?.length) out.push('Headers');
  if (o.compress) out.push('Compressed');
  if (o.strip_prefix && d.path !== '/') out.push('Strip prefix');
  if (o.redirect_www) out.push('www redirect');
  if (d.advanced) out.push('Advanced');
  return out;
}

function ReplaceCertificate({
  domain,
  shared,
  onClose,
  onDone,
}: {
  domain: Domain | null;
  shared: boolean;
  onClose: () => void;
  onDone: () => void;
}) {
  const [cert, setCert] = useState<CertificateValue>(emptyCertificate);
  const upload = useMutation({
    mutationFn: () => {
      const form = new FormData();
      appendCertificate(form, cert);
      return api.put(`/domains/${domain!.id}/certificate`, form);
    },
    onSuccess: () => {
      notifications.show({ color: 'green', message: `Certificate replaced for ${domain!.host}` });
      setCert(emptyCertificate);
      onDone();
      onClose();
    },
    onError: (err) => notifications.show({ color: 'red', message: err.message }),
  });

  return (
    <Modal opened={domain !== null} onClose={onClose} title={`Certificate for ${domain?.host}`} size="lg">
      <Stack>
        {domain?.certificate && <CertificateSummary cert={domain.certificate} />}
        {shared && (
          <Text size="sm" c="dimmed">
            Every path on {domain?.host} uses this certificate.
          </Text>
        )}
        <CertificateFields value={cert} onChange={setCert} />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={!certificateReady(cert)} loading={upload.isPending} onClick={() => upload.mutate()}>
            Upload
          </Button>
        </Group>
      </Stack>
    </Modal>
  );
}
