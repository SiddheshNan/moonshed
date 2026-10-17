import { Badge, Box, Tooltip } from '@mantine/core';

import type { App, DeployStatus } from '../types';

type Look = { color: string; label: string; live?: boolean };

// appLook is the one status shown for an app everywhere in the UI.
export function appLook(app: App): Look {
  if (app.status.state === 'running') {
    return app.health === 'down' ? { color: 'orange', label: 'Unhealthy', live: true } : { color: 'green', label: 'Running', live: true };
  }
  if (app.status.state === 'crashed') return { color: 'red', label: 'Crashed' };
  const last = app.last_deployment;
  if (last && isActive(last.status)) return { color: 'blue', label: 'Deploying', live: true };
  if (last?.status === 'failed') return { color: 'red', label: 'Deploy failed' };
  if (!app.current_release_id) return { color: 'gray', label: 'Not deployed' };
  return { color: 'gray', label: 'Stopped' };
}

export function Dot({ color, live }: { color: string; live?: boolean }) {
  return (
    <Box
      className="status-dot"
      data-live={live || undefined}
      style={{ '--dot': `var(--mantine-color-${color}-${color === 'gray' ? 5 : 6})` } as React.CSSProperties}
    />
  );
}

export function AppStatusBadge({ app }: { app: App }) {
  const look = appLook(app);
  return (
    <Badge color={look.color} leftSection={<Dot color={look.color} />}>
      {look.label}
    </Badge>
  );
}

export function StatusDot({ app }: { app: App }) {
  const look = appLook(app);
  return (
    <Tooltip label={look.label} position="right">
      <Dot color={look.color} live={look.live} />
    </Tooltip>
  );
}

const deployColors: Record<DeployStatus, string> = {
  queued: 'gray',
  building: 'blue',
  starting: 'cyan',
  success: 'green',
  failed: 'red',
  killed: 'orange',
};

export const deployLabels: Record<DeployStatus, string> = {
  queued: 'Queued',
  building: 'Building',
  starting: 'Starting',
  success: 'Succeeded',
  failed: 'Failed',
  killed: 'Killed',
};

export function DeployStatusBadge({ status }: { status: DeployStatus }) {
  return (
    <Badge color={deployColors[status]} leftSection={<Dot color={deployColors[status]} live={isActive(status)} />}>
      {deployLabels[status]}
    </Badge>
  );
}

export function isActive(status: DeployStatus) {
  return status === 'queued' || status === 'building' || status === 'starting';
}
