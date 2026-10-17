import { summarizeError } from './format';
import type { App } from './types';

// appProblem explains what is wrong with an app, or returns null when nothing is.
export function appProblem(app: App): string | null {
  if (app.status.state === 'crashed') {
    return app.status.restarts > 0 ? `Crashed, restarted ${app.status.restarts} times` : 'Crashed';
  }
  if (app.status.state === 'running' && app.health === 'down') return 'Failing its health check';
  if (app.status.state !== 'running' && app.desired_state === 'running' && app.current_release_id) return 'Stopped unexpectedly';
  const last = app.last_deployment;
  if (last?.status === 'failed') return `Last deployment failed${last.error ? `: ${summarizeError(last.error)}` : ''}`;
  return null;
}

// byProblem sorts apps with a problem first, then by name.
export function byProblem(apps: App[]): { app: App; problem: string | null }[] {
  return apps
    .map((app) => ({ app, problem: appProblem(app) }))
    .sort((a, b) => Number(!a.problem) - Number(!b.problem) || a.app.name.localeCompare(b.app.name));
}
