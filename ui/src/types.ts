export type AppState = 'running' | 'stopped' | 'crashed' | 'starting';
export type Runtime = '' | 'native' | 'docker';
export type Source = 'upload' | 'git';
export type TLSMode = 'none' | 'letsencrypt' | 'custom';

export interface User {
  id: string;
  username: string;
  created_at: string;
}

export interface AuthStatus {
  setup_required: boolean;
  version: string;
  build: string;
  user?: User;
}

export interface Status {
  state: AppState;
  pid?: number;
  release_id?: string;
  since?: string;
  restarts: number;
  exit_code: number;
}

export interface AppStats {
  cpu: number;
  memory: number;
  processes: number;
  disk: number;
}

export interface CertInfo {
  names: string[];
  issuer: string;
  not_after: string;
}

export interface HeaderRule {
  op: 'set' | 'add' | 'remove';
  name: string;
  value: string;
}

// Proxy options for one domain. force_https, hsts and redirect_www apply to every path on the host.
export interface DomainOptions {
  force_https: boolean;
  hsts: boolean;
  redirect_www: boolean;
  compress: boolean;
  strip_prefix: boolean;
  basic_auth: { user: string }[] | null;
  allow_ips: string[] | null;
  request_headers: HeaderRule[] | null;
  response_headers: HeaderRule[] | null;
  max_body: string;
}

export interface Domain {
  id: string;
  app_id: string;
  host: string;
  path: string;
  tls: TLSMode;
  options: DomainOptions;
  advanced: string;
  certificate: CertInfo | null;
}

export interface ProxyCheck {
  ok: boolean;
  error?: string;
  caddyfile: string;
}

export interface App {
  id: string;
  project_id: string;
  name: string;
  runtime: Runtime;
  port: number;
  source: Source;
  git_url: string;
  git_branch: string;
  git_subdir: string;
  git_credential_id: string;
  github_installation_id: number;
  autodeploy: boolean;
  pushed_commit: string;
  pushed_message: string;
  record_requests: boolean;
  run_as_user: string;
  keep_bundles: number;
  log_retention_days: number;
  backup_enabled: boolean;
  backup_cron: string;
  backup_keep: number;
  backup_parts: BackupParts;
  current_release_id: string;
  desired_state: 'running' | 'stopped';
  created_at: string;
  status: Status;
  stats: AppStats;
  health: Health;
  health_check: string;
  deployed: Deployment | null;
  last_deployment: Deployment | null;
  has_run_as_password: boolean;
  domains: Domain[];
  webhook_secret?: string;
  config: Overrides;
  file: Manifest | null;
}

// moonshed.yaml, as found in the running release.
export interface Manifest {
  pre: Script;
  run: Script;
  post: Script;
  persist: string[] | null;
  port?: number;
  health?: HealthCheck;
  restart_unhealthy?: boolean;
  build?: BuildConfig;
}

export interface Script {
  linux?: string;
  windows?: string;
}

// Durations are Go-style strings like "30s" or "1m0s".
export interface HealthCheck {
  http?: string;
  tcp?: boolean;
  interval?: string;
  timeout?: string;
  failures?: number;
  grace?: string;
}

export interface BuildConfig {
  dockerfile?: string;
  folder?: string;
  stage?: string;
  args?: Record<string, string>;
}

// Settings changed in the UI on top of moonshed.yaml. A missing field falls back to the file.
export interface Overrides {
  pre?: string;
  run?: string;
  post?: string;
  persist?: string[];
  health?: HealthCheck;
  restart_unhealthy?: boolean;
  build?: BuildConfig;
}

export type Health = 'up' | 'down' | 'unknown';

export type DeployStatus = 'queued' | 'building' | 'starting' | 'success' | 'failed' | 'killed';

export interface Deployment {
  id: string;
  app_id: string;
  bundle_id: string;
  release_id: string;
  trigger: string;
  git_commit: string;
  git_message: string;
  status: DeployStatus;
  error: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface Bundle {
  id: string;
  app_id: string;
  source: Source;
  git_commit: string;
  size: number;
  created_at: string;
}

export interface Backup {
  id: string;
  app_id: string;
  file: string;
  size: number;
  status: 'running' | 'success' | 'failed';
  error: string;
  parts: BackupParts;
  created_at: string;
}

// What an app backup includes. Platform backups always hold the database and its key.
export interface BackupParts {
  code: boolean;
  data: boolean;
  env: boolean;
  logs: boolean;
}

export interface Incident {
  id: string;
  app_id: string;
  down_at: string;
  up_at: string | null;
  reason: string;
}

export interface Uptime {
  uptime_24h: number | null;
  uptime_7d: number | null;
  uptime_30d: number | null;
  bars: (number | null)[];
  bars_start: string;
  bar_span: number;
  incidents: Incident[];
  health: Health;
}

export interface Point {
  t: number;
  v: number;
  max?: number;
}

export type Series = Record<string, Point[]>;

export interface AppMetrics {
  app: Series;
  traffic: Record<string, Series>;
}

export interface ServerStats {
  hostname: string;
  os: string;
  cores: number;
  uptime: number;
  cpu: number;
  mem_used: number;
  mem_total: number;
  disk_used: number;
  disk_total: number;
  net_in: number;
  net_out: number;
  load1: number;
}

export interface ProxyStatus {
  enabled: boolean;
  listening: boolean;
  http_addr: string;
  https_addr: string;
  error?: string;
}

export interface ServerInfo {
  version: string;
  build: string;
  stats: ServerStats | null;
  docker: boolean;
  docker_version: string;
  proxy: ProxyStatus;
  panel_addr: string;
  runtime_dir: string;
}

export interface SettingsResponse {
  settings: Settings;
  proxy: ProxyStatus;
  panel_addr: string;
  config_file: string;
  panel_certificate?: CertInfo | null;
  error?: string;
}

export interface Settings {
  proxy_enabled: boolean;
  acme_email: string;
  panel_domain: string;
  panel_tls: TLSMode;
  trusted_proxies: string[];
  global_options: string;
  extra_sites: string;
  backup_enabled: boolean;
  backup_cron: string;
  backup_keep: number;
}

export interface APIToken {
  id: string;
  user_id: string;
  name: string;
  last_used_at: string | null;
  created_at: string;
}

export interface Project {
  id: string;
  name: string;
  description: string;
  created_at: string;
}

export interface ProjectSummary extends Project {
  apps: number;
  running: number;
  unhealthy: number;
}

export interface DeploymentRow extends Deployment {
  app_name: string;
  project_id: string;
  git_url: string;
}

export interface AuditEntry {
  seq: number;
  time: string;
  actor_type: 'user' | 'token' | 'webhook' | 'system';
  actor_id: string;
  actor_name: string;
  ip: string;
  action: string;
  target_type: string;
  target_id: string;
  target_name: string;
  details: string;
}

export interface RequestEntry {
  id: number;
  app_id: string;
  time: number;
  method: string;
  host: string;
  path: string;
  query: string;
  status: number;
  duration_ms: number;
  bytes: number;
  ip: string;
  user_agent: string;
  referer: string;
}

export interface RecordingStatus {
  active: boolean;
  proxy: boolean;
  app_ids: string[];
  counts: Record<string, number>;
  cap: number;
}

export interface GitCredential {
  id: string;
  name: string;
  host: string;
  username: string;
  created_at: string;
}

export interface GitHubApp {
  id: number;
  slug: string;
  name: string;
  owner: string;
  html_url: string;
  created_at: string;
}

export interface GitHubInstallation {
  id: number;
  github_app_id: number;
  account: string;
  account_type: string;
  created_at: string;
}

export interface GitOverview {
  credentials: GitCredential[];
  github_apps: GitHubApp[];
  github_installations: GitHubInstallation[];
}

export interface Repo {
  full_name: string;
  clone_url: string;
  default_branch: string;
  private: boolean;
}
