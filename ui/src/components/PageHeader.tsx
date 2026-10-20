import type { ReactNode } from 'react';
import { Anchor, Breadcrumbs, Group, Stack, Text, Title } from '@mantine/core';
import { Link } from 'react-router';

export type Crumb = { label: string; to?: string };

export function PageHeader({
  crumbs,
  title,
  badges,
  description,
  actions,
}: {
  crumbs?: Crumb[];
  title: ReactNode;
  badges?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <Stack gap={6}>
      {crumbs && crumbs.length > 0 && (
        <Breadcrumbs separatorMargin={6}>
          {crumbs.map((c, i) =>
            c.to ? (
              <Anchor key={i} component={Link} to={c.to} size="sm" c="dimmed">
                {c.label}
              </Anchor>
            ) : (
              <Text key={i} size="sm" c="dimmed">
                {c.label}
              </Text>
            ),
          )}
        </Breadcrumbs>
      )}
      <Group justify="space-between" align="flex-start" wrap="wrap" gap="sm">
        <div style={{ minWidth: 0, maxWidth: '100%' }}>
          <Group gap="sm">
            <Title order={2}>{title}</Title>
            {badges}
          </Group>
          {description && (
            <Text size="sm" c="dimmed" mt={4} truncate>
              {description}
            </Text>
          )}
        </div>
        {actions && <Group gap="xs">{actions}</Group>}
      </Group>
    </Stack>
  );
}
