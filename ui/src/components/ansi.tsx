import type { CSSProperties, ReactNode } from 'react';

// Terminal colors tuned for the always-dark log viewer.
const palette = ['#5d5f68', '#f87171', '#4ade80', '#facc15', '#60a5fa', '#c084fc', '#22d3ee', '#d4d4d8'];
const bright = ['#8b8d98', '#fca5a5', '#86efac', '#fde047', '#93c5fd', '#d8b4fe', '#67e8f9', '#fafafa'];

type Style = { color?: string; bold?: boolean; dim?: boolean };

function apply(style: Style, codes: number[]): Style {
  let next = { ...style };
  for (const code of codes.length ? codes : [0]) {
    if (code === 0) next = {};
    else if (code === 1) next.bold = true;
    else if (code === 2) next.dim = true;
    else if (code === 22) next.bold = next.dim = false;
    else if (code >= 30 && code <= 37) next.color = palette[code - 30];
    else if (code >= 90 && code <= 97) next.color = bright[code - 90];
    else if (code === 39) next.color = undefined;
  }
  return next;
}

// ansi turns text with SGR escape codes (colors, bold) into styled spans and drops other escapes.
export function ansi(text: string): ReactNode {
  if (!text.includes('\x1b')) return text;
  const out: ReactNode[] = [];
  let style: Style = {};
  let last = 0;
  const re = /\x1b\[([\d;]*)([A-Za-z])/g;
  for (let m = re.exec(text); m; m = re.exec(text)) {
    if (m.index > last) out.push(span(text.slice(last, m.index), style, out.length));
    if (m[2] === 'm') style = apply(style, m[1].split(';').filter(Boolean).map(Number));
    last = re.lastIndex;
  }
  if (last < text.length) out.push(span(text.slice(last), style, out.length));
  return out;
}

function span(text: string, style: Style, key: number) {
  if (!style.color && !style.bold && !style.dim) return text;
  const css: CSSProperties = { color: style.color, fontWeight: style.bold ? 600 : undefined, opacity: style.dim ? 0.6 : undefined };
  return (
    <span key={key} style={css}>
      {text}
    </span>
  );
}
