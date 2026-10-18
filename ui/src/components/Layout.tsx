import type { ReactNode } from 'react';
import {
  ActionIcon,
  AppShell,
  Avatar,
  Badge,
  Burger,
  Group,
  Kbd,
  NavLink,
  ScrollArea,
  Stack,
  Text,
  Tooltip,
  UnstyledButton,
  useComputedColorScheme,
  useMantineColorScheme,
} from '@mantine/core';
import { useDisclosure } from '@mantine/hooks';
import { Spotlight, spotlight, type SpotlightActionData } from '@mantine/spotlight';
import {
  IconCertificate,
  IconFolder,
  IconGitBranch,
  IconKey,
  IconLayoutDashboard,
  IconListDetails,
  IconLogout,
  IconMoon,
  IconRocket,
  IconSearch,
  IconSettings,
  IconSun,
  IconTransfer,
  IconUsers,
  IconWorld,
} from '@tabler/icons-react';
import { useQuery } from '@tanstack/react-query';
import { Link, useLocation, useNavigate } from 'react-router';

import { api, queryClient } from '../api';
import { appPath } from '../paths';
import type { App, Project, User } from '../types';
import { StatusDot } from './StatusBadge';

export function Logo({ size = 'sm' }: { size?: 'sm' | 'lg' }) {
  return (
    <svg className="app-logo" data-size={size} viewBox="0 0 64 64" role="img" aria-label="Moonshed">
      <defs>
        <linearGradient id="ms-sky" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#1c2347" />
          <stop offset="1" stopColor="#090c16" />
        </linearGradient>
        <linearGradient id="ms-moon" x1="0.2" y1="0" x2="0.8" y2="1">
          <stop offset="0" stopColor="#ffe28f" />
          <stop offset="1" stopColor="#f5b02a" />
        </linearGradient>
      </defs>
      <rect width="64" height="64" rx="16" fill="url(#ms-sky)" />
      <circle cx="42" cy="22" r="12" fill="url(#ms-moon)" />
      <path d="M12 54V35.5L52 27v27z" fill="#2c3563" />
      <path d="M12 35.5L52 27" stroke="#3b4684" strokeWidth="2" strokeLinecap="round" />
      <rect x="21" y="40" width="9" height="9" rx="2" fill="#ffd35e" />
    </svg>
  );
}

const home = [
  { to: '/', label: 'Dashboard', icon: IconLayoutDashboard, exact: true },
  { to: '/projects', label: 'Projects', icon: IconFolder },
  { to: '/deployments', label: 'Deployments', icon: IconRocket },
  { to: '/requests', label: 'Requests', icon: IconTransfer },
];

const settings = [
  { to: '/settings', label: 'General', icon: IconSettings, exact: true },
  { to: '/settings/proxy', label: 'Proxy', icon: IconWorld },
  { to: '/settings/git', label: 'Git', icon: IconGitBranch },
  { to: '/settings/certificates', label: 'Certificates', icon: IconCertificate },
  { to: '/settings/users', label: 'Users', icon: IconUsers },
  { to: '/settings/tokens', label: 'API tokens', icon: IconKey },
  { to: '/settings/audit', label: 'Audit log', icon: IconListDetails },
];

function NavSection({ title, items, onNavigate }: { title: string; items: typeof home; onNavigate: () => void }) {
  const location = useLocation();
  return (
    <Stack gap={2}>
      <Text className="nav-heading" px="sm" mb={6}>
        {title}
      </Text>
      {items.map((item) => (
        <NavLink
          key={item.to}
          component={Link}
          to={item.to}
          label={item.label}
          leftSection={<item.icon size={18} stroke={1.7} />}
          active={item.exact ? location.pathname === item.to : location.pathname.startsWith(item.to)}
          onClick={onNavigate}
        />
      ))}
    </Stack>
  );
}

// Search lets you jump to any app, project or page, which is how you navigate with hundreds of apps.
function Search() {
  const navigate = useNavigate();
  const apps = useQuery({ queryKey: ['apps'], queryFn: () => api.get<App[]>('/apps') });
  const projects = useQuery({ queryKey: ['projects-list'], queryFn: () => api.get<Project[]>('/projects') });
  const projectName = new Map(projects.data?.map((p) => [p.id, p.name]));
  const actions: SpotlightActionData[] = [
    ...(apps.data ?? []).map((app) => ({
      id: `app-${app.id}`,
      label: app.name,
      description: `App in ${projectName.get(app.project_id) ?? 'project'} · ${app.runtime}`,
      leftSection: <StatusDot app={app} />,
      onClick: () => navigate(appPath(app.project_id, app.id)),
    })),
    ...(projects.data ?? []).map((p) => ({
      id: `project-${p.id}`,
      label: p.name,
      description: 'Project',
      leftSection: <IconFolder size={16} />,
      onClick: () => navigate(`/projects/${p.id}`),
    })),
    ...[...home, ...settings].map((item) => ({
      id: `page-${item.to}`,
      label: item.label,
      description: item.to.startsWith('/settings') && item.to !== '/settings' ? 'Settings' : 'Page',
      leftSection: <item.icon size={16} />,
      onClick: () => navigate(item.to),
    })),
  ];
  return (
    <Spotlight
      actions={actions}
      shortcut={['mod + K', '/']}
      limit={12}
      nothingFound="Nothing found"
      highlightQuery
      searchProps={{ leftSection: <IconSearch size={18} />, placeholder: 'Search apps, projects, pages…' }}
    />
  );
}

export function Layout({ user, version, children }: { user: User; version: string; children: ReactNode }) {
  const [opened, { toggle, close }] = useDisclosure();
  const { setColorScheme } = useMantineColorScheme();
  const scheme = useComputedColorScheme('dark');

  const logout = async () => {
    await api.post('/auth/logout');
    queryClient.clear();
    queryClient.invalidateQueries({ queryKey: ['auth'] });
  };

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 248, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="xl"
    >
      <AppShell.Header className="app-chrome app-header">
        <Group h="100%" px="md" justify="space-between" wrap="nowrap">
          <Group gap="sm" wrap="nowrap">
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Logo />
            <Text fw={650} size="md" style={{ letterSpacing: '-0.01em' }}>
              Moonshed
            </Text>
            <Badge variant="default" size="sm" radius="xl" fw={500} c="dimmed" visibleFrom="xs">
              {version}
            </Badge>
          </Group>
          <Group gap={6} wrap="nowrap">
            <UnstyledButton className="search-button" onClick={() => spotlight.open()}>
              <Group gap={8} wrap="nowrap">
                <IconSearch size={15} />
                <Text size="sm" c="dimmed" visibleFrom="xs">
                  Search
                </Text>
                <Kbd size="xs" visibleFrom="xs">
                  ⌘K
                </Kbd>
              </Group>
            </UnstyledButton>
            <Tooltip label={scheme === 'dark' ? 'Light mode' : 'Dark mode'}>
              <ActionIcon size="lg" onClick={() => setColorScheme(scheme === 'dark' ? 'light' : 'dark')}>
                {scheme === 'dark' ? <IconSun size={18} /> : <IconMoon size={18} />}
              </ActionIcon>
            </Tooltip>
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar p="sm" className="app-chrome">
        <AppShell.Section grow component={ScrollArea}>
          <Stack gap="xl" pt={4}>
            <NavSection title="Home" items={home} onNavigate={close} />
            <NavSection title="Settings" items={settings} onNavigate={close} />
          </Stack>
        </AppShell.Section>
        <AppShell.Section pt="sm" mt="sm" style={{ borderTop: '1px solid var(--app-border)' }}>
          <Group justify="space-between" px={4} wrap="nowrap">
            <Group gap="sm" wrap="nowrap">
              <Avatar size={30} radius="xl" color="indigo" variant="light">
                {user.username.slice(0, 2).toUpperCase()}
              </Avatar>
              <Text size="sm" fw={500} truncate>
                {user.username}
              </Text>
            </Group>
            <Tooltip label="Log out">
              <ActionIcon onClick={logout}>
                <IconLogout size={17} />
              </ActionIcon>
            </Tooltip>
          </Group>
        </AppShell.Section>
      </AppShell.Navbar>

      <AppShell.Main>{children}</AppShell.Main>
      <Search />
    </AppShell>
  );
}
