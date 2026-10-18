import { Tooltip } from '@mantine/core';

import { formatDateTime, timeAgo } from '../format';

// TimeAgo shows a relative time ("5m ago") with the exact date and time on hover.
export function TimeAgo({ iso, prefix = '', suffix = true }: { iso: string | null | undefined; prefix?: string; suffix?: boolean }) {
  if (!iso) return null;
  const text = suffix ? timeAgo(iso) : timeAgo(iso).replace(' ago', '');
  return (
    <Tooltip label={formatDateTime(iso)}>
      <span>
        {prefix}
        {text}
      </span>
    </Tooltip>
  );
}
