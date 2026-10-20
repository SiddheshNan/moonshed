import { Anchor, Code, Tooltip } from '@mantine/core';

import { commitURL } from '../paths';

// Commit shows a short commit hash, linked to the commit on GitHub or GitLab when the repo is there.
export function Commit({ gitUrl, sha, message }: { gitUrl: string; sha: string; message?: string }) {
  const url = commitURL(gitUrl, sha);
  const code = <Code>{sha.slice(0, 7)}</Code>;
  return (
    <Tooltip label={message || sha} disabled={!message && !url} multiline maw={480} style={{ whiteSpace: 'pre-wrap' }}>
      {url ? (
        <Anchor href={url} target="_blank" rel="noreferrer" onClick={(e) => e.stopPropagation()}>
          {code}
        </Anchor>
      ) : (
        code
      )}
    </Tooltip>
  );
}
