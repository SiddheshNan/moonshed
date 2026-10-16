import { useEffect, useRef, useState } from 'react';
import { Badge, Box, Group, ScrollArea, Switch, Text } from '@mantine/core';

const maxLines = 5000;

// LogViewer follows a server-sent events stream of log lines.
export function LogViewer({ url, height = 520 }: { url: string; height?: number | string }) {
  const [lines, setLines] = useState<string[]>([]);
  const [live, setLive] = useState(true);
  const [follow, setFollow] = useState(true);
  const viewport = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setLines([]);
    setLive(true);
    const source = new EventSource(url);
    let buffer: string[] = [];
    const flush = setInterval(() => {
      if (buffer.length === 0) return;
      const batch = buffer;
      buffer = [];
      setLines((prev) => {
        const next = prev.concat(batch);
        return next.length > maxLines ? next.slice(next.length - maxLines) : next;
      });
    }, 150);
    source.onmessage = (e) => buffer.push(e.data);
    source.addEventListener('end', () => {
      source.close();
      setLive(false);
    });
    source.onerror = () => {
      if (source.readyState === EventSource.CLOSED) setLive(false);
    };
    return () => {
      clearInterval(flush);
      source.close();
    };
  }, [url]);

  useEffect(() => {
    if (follow && viewport.current) {
      viewport.current.scrollTo({ top: viewport.current.scrollHeight });
    }
  }, [lines, follow]);

  return (
    <Box>
      <Group justify="space-between" mb="xs">
        <Badge variant="dot" color={live ? 'green' : 'gray'}>
          {live ? 'Live' : 'Ended'}
        </Badge>
        <Switch size="xs" label="Follow" checked={follow} onChange={(e) => setFollow(e.currentTarget.checked)} />
      </Group>
      <ScrollArea
        h={height}
        viewportRef={viewport}
        className="terminal"
        p="md"
      >
        {lines.length === 0 && (
          <Text c="dimmed" ff="monospace" fz="xs">
            Waiting for output…
          </Text>
        )}
        {lines.map((line, i) => (
          <LogLine key={i} line={line} />
        ))}
      </ScrollArea>
    </Box>
  );
}

function LogLine({ line }: { line: string }) {
  const space = line.indexOf(' ');
  const time = space > 0 ? line.slice(11, 19) : '';
  const text = space > 0 ? line.slice(space + 1) : line;
  const platform = text.startsWith('==> ');
  return (
    <Text
      component="div"
      ff="monospace"
      fz="xs"
      c={platform ? 'indigo.3' : undefined}
      fw={platform ? 600 : 400}
      style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all', lineHeight: 1.55 }}
    >
      <Text span inherit c="#5d5f68" fw={400} mr="md">
        {time}
      </Text>
      {platform ? text.slice(4) : text}
    </Text>
  );
}
