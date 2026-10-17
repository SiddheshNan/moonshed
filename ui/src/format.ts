export function formatBytes(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1);
  return `${(n / 1024 ** i).toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

// formatBytesShort drops decimals once a value has three digits, for chart axes.
export function formatBytesShort(n: number): string {
  if (!n) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(Math.abs(n)) / Math.log(1024)), units.length - 1);
  const v = n / 1024 ** Math.max(i, 0);
  return `${v >= 100 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[Math.max(i, 0)]}`;
}

export function formatPercent(n: number, digits = 1): string {
  return `${n.toFixed(digits)}%`;
}

// formatSmallPercent keeps a few significant digits for values well under 1%.
export function formatSmallPercent(n: number): string {
  if (n === 0) return '0%';
  if (n < 1) return `${n.toPrecision(2)}%`;
  return `${n.toFixed(1)}%`;
}

export function timeAgo(iso: string | null | undefined): string {
  if (!iso) return '—';
  const seconds = Math.round((Date.now() - new Date(iso).getTime()) / 1000);
  if (seconds < 5) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`;
  return `${Math.floor(seconds / 86400)}d ago`;
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`;
}

export function elapsed(from: string | null, to: string | null): string {
  if (!from) return '—';
  const end = to ? new Date(to).getTime() : Date.now();
  return formatDuration((end - new Date(from).getTime()) / 1000);
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—';
  return new Date(iso).toLocaleString();
}

// summarizeError keeps the first line of an error, short enough for a table cell or a banner.
export function summarizeError(err: string, max = 140): string {
  const line = err.split('\n')[0].trim();
  return line.length > max ? `${line.slice(0, max - 1)}…` : line;
}

const triggerLabels: Record<string, string> = {
  upload: 'Zip upload',
  api: 'API token',
  manual: 'Manual',
  github_app: 'GitHub push',
  github_webhook: 'GitHub webhook',
  gitlab_webhook: 'GitLab webhook',
  deploy_hook: 'Deploy hook',
  redeploy: 'Redeploy',
};

// triggerLabel names what started a deployment.
export function triggerLabel(trigger: string): string {
  return triggerLabels[trigger] ?? trigger;
}

// runtimeLabel names an app's runtime, or '' before it's detected.
export function runtimeLabel(runtime: string): string {
  return runtime === 'docker' ? 'Docker' : runtime === 'native' ? 'Native' : '';
}

// sourceLabel says where an app's versions come from: owner/repo for git, else zip upload.
export function sourceLabel(app: { source: string; git_url: string }): string {
  if (app.source !== 'git') return 'Zip upload';
  const path = app.git_url.replace(/^(?:https?:\/\/(?:[^@/]+@)?[^/]+\/|[^@]+@[^:]+:)/, '').replace(/\.git$/, '').replace(/\/$/, '');
  return path || app.git_url;
}
