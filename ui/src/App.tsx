import { Center, Loader } from '@mantine/core';
import { useQuery } from '@tanstack/react-query';
import { Navigate, Route, Routes } from 'react-router';

import { api } from './api';
import { Layout } from './components/Layout';
import { OfflineBanner, Unreachable } from './components/Offline';
import { AppPage } from './pages/app/AppPage';
import { AppsPage } from './pages/AppsPage';
import { Dashboard } from './pages/Dashboard';
import { DeploymentsPage } from './pages/DeploymentsPage';
import { Login } from './pages/Login';
import { ProjectPage } from './pages/ProjectPage';
import { ProjectsPage } from './pages/Projects';
import { RequestsPage } from './pages/RequestsPage';
import { AuditPage } from './pages/settings/Audit';
import { CertificatesPage } from './pages/settings/Certificates';
import { BackupsSettingsPage } from './pages/settings/Backups';
import { GitSettingsPage } from './pages/settings/Git';
import { ProxySettingsPage } from './pages/settings/Proxy';
import { TokensPage } from './pages/settings/Tokens';
import { UsersPage } from './pages/settings/Users';
import type { AuthStatus } from './types';

export function App() {
  const auth = useQuery({ queryKey: ['auth'], queryFn: () => api.get<AuthStatus>('/auth/status') });

  if (auth.isPending) {
    return (
      <Center h="100vh">
        <Loader />
      </Center>
    );
  }
  if (!auth.data) {
    return <Unreachable />;
  }
  if (!auth.data.user) {
    return (
      <>
        <OfflineBanner />
        <Login setup={auth.data.setup_required} setupCommand={auth.data.setup_command} />
      </>
    );
  }

  return (
    <Layout user={auth.data.user} version={auth.data.version} build={auth.data.build}>
      <OfflineBanner />
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/projects/:id" element={<ProjectPage />} />
        <Route path="/projects/:projectId/apps/:id/*" element={<AppPage />} />
        <Route path="/apps" element={<AppsPage />} />
        <Route path="/deployments" element={<DeploymentsPage />} />
        <Route path="/requests" element={<RequestsPage />} />
        <Route path="/settings" element={<Navigate to="/settings/proxy" replace />} />
        <Route path="/settings/proxy" element={<ProxySettingsPage />} />
        <Route path="/settings/git" element={<GitSettingsPage />} />
        <Route path="/settings/certificates" element={<CertificatesPage />} />
        <Route path="/settings/users" element={<UsersPage />} />
        <Route path="/settings/tokens" element={<TokensPage />} />
        <Route path="/settings/audit" element={<AuditPage />} />
        <Route path="/settings/backups" element={<BackupsSettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  );
}
