import { Anchor, Card, Stack, Table, Text } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';

import { api } from '../../api';
import { CertificateSummary } from '../../components/Certificate';
import { PageHeader } from '../../components/PageHeader';
import { tlsLabels } from '../../components/TLSPicker';
import { appPath } from '../../paths';
import type { App, CertInfo, SettingsResponse, TLSMode } from '../../types';

type Row = { host: string; tls: TLSMode; usedBy: { label: string; to: string }[]; cert: CertInfo | null };

export function CertificatesPage() {
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps') });
  const settings = useQuery({ queryKey: ['settings'], queryFn: () => api.get<SettingsResponse>('/settings') });

  const rows = new Map<string, Row>();
  for (const app of apps.data ?? []) {
    for (const d of app.domains) {
      if (d.tls === 'none') continue;
      const row = rows.get(d.host) ?? { host: d.host, tls: d.tls, usedBy: [], cert: d.certificate };
      row.usedBy.push({ label: `${app.name}${d.path === '/' ? '' : d.path}`, to: appPath(app.project_id, app.id, 'domains') });
      rows.set(d.host, row);
    }
  }
  const s = settings.data;
  if (s?.settings.panel_domain && s.settings.panel_tls !== 'none') {
    const { panel_domain: host, panel_tls: tls } = s.settings;
    rows.set(host, { host, tls, usedBy: [{ label: 'Panel', to: '/settings/proxy' }], cert: s.panel_certificate ?? null });
  }
  const list = [...rows.values()].sort((a, b) => a.host.localeCompare(b.host));

  return (
    <Stack gap="lg">
      <PageHeader crumbs={[{ label: 'Settings' }]} title="Certificates" />
      <Card padding={0}>
        {list.length === 0 ? (
          <Text c="dimmed" size="sm" p="lg">
            No domain uses HTTPS yet.
          </Text>
        ) : (
          <Table.ScrollContainer minWidth={640}>
            <Table>
              <Table.Thead>
                <Table.Tr>
                  <Table.Th>Domain</Table.Th>
                  <Table.Th>Type</Table.Th>
                  <Table.Th>Used by</Table.Th>
                  <Table.Th>Certificate</Table.Th>
                </Table.Tr>
              </Table.Thead>
              <Table.Tbody>
                {list.map((r) => (
                  <Table.Tr key={r.host}>
                    <Table.Td fw={500}>{r.host}</Table.Td>
                    <Table.Td className="nowrap">{tlsLabels[r.tls]}</Table.Td>
                    <Table.Td>
                      <Stack gap={2}>
                        {r.usedBy.map((u) => (
                          <Anchor key={u.label} component={Link} to={u.to} size="sm">
                            {u.label}
                          </Anchor>
                        ))}
                      </Stack>
                    </Table.Td>
                    <Table.Td maw={380}>
                      <CertificateSummary cert={r.cert} missing={r.tls === 'letsencrypt' ? 'Not issued yet' : undefined} />
                    </Table.Td>
                  </Table.Tr>
                ))}
              </Table.Tbody>
            </Table>
          </Table.ScrollContainer>
        )}
      </Card>
    </Stack>
  );
}
