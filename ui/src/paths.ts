// appPath is an app's page, or one of its tabs: /projects/<project>/apps/<app>/<tab>.
export function appPath(projectId: string, appId: string, tab = ''): string {
  const base = `/projects/${projectId}/apps/${appId}`;
  return tab ? `${base}/${tab}` : base;
}

// commitURL links a commit on GitHub or GitLab. Other hosts have no URL scheme we can rely on.
export function commitURL(gitURL: string, sha: string): string | null {
  const m = gitURL.trim().match(/^(?:https?:\/\/(?:[^@/]+@)?|git@)(github\.com|gitlab\.com)[/:](.+?)(?:\.git)?\/?$/i);
  if (!m || !sha) return null;
  const [, host, repo] = m;
  return host.toLowerCase() === 'github.com' ? `https://github.com/${repo}/commit/${sha}` : `https://gitlab.com/${repo}/-/commit/${sha}`;
}
