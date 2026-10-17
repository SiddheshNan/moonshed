import {
  ActionIcon,
  AppShell,
  Badge,
  Button,
  Card,
  createTheme,
  Modal,
  NavLink,
  Paper,
  Progress,
  SegmentedControl,
  Table,
  Tabs,
  Tooltip,
  type CSSVariablesResolver,
  type MantineColorsTuple,
} from '@mantine/core';

// Neutral (zinc) greys so the only colour on screen is meaning: indigo for actions, green/orange/red for state.
// Mantine reads dark as: 0-2 text, 3 dimmed, 4 borders, 5 hover, 6 surfaces, 7 page.
const dark: MantineColorsTuple = [
  '#ededef',
  '#c9c9ce',
  '#a0a0a8',
  '#7b7b84',
  '#2a2a2f',
  '#222226',
  '#18181b',
  '#111113',
  '#0c0c0e',
  '#08080a',
];

const gray: MantineColorsTuple = [
  '#fafafa',
  '#f4f4f5',
  '#e9e9ec',
  '#dcdce0',
  '#c4c4ca',
  '#a1a1aa',
  '#71717a',
  '#52525b',
  '#3f3f46',
  '#27272a',
];

export const theme = createTheme({
  colors: { dark, gray },
  primaryColor: 'indigo',
  primaryShade: { light: 6, dark: 5 },
  autoContrast: true,
  defaultRadius: 'md',
  cursorType: 'pointer',
  fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI Variable Text", "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif',
  fontFamilyMonospace: '"JetBrains Mono", ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
  radius: { xs: '4px', sm: '6px', md: '8px', lg: '10px', xl: '14px' },
  headings: {
    fontWeight: '600',
    sizes: {
      h2: { fontSize: '1.375rem', lineHeight: '1.3' },
      h3: { fontSize: '1.125rem', lineHeight: '1.35' },
    },
  },
  components: {
    AppShell: AppShell.extend({
      styles: { main: { background: 'var(--app-page)' } },
    }),
    Card: Card.extend({
      defaultProps: { withBorder: true, radius: 'md' },
      styles: { root: { background: 'var(--app-surface)', borderColor: 'var(--app-border)' } },
    }),
    Paper: Paper.extend({
      defaultProps: { radius: 'md' },
    }),
    Badge: Badge.extend({
      defaultProps: { variant: 'light', radius: 'sm', size: 'md' },
      styles: { root: { textTransform: 'none', fontWeight: 500, letterSpacing: 0, overflow: 'visible' }, label: { overflow: 'visible' } },
    }),
    Button: Button.extend({
      defaultProps: { radius: 'md' },
      styles: { root: { fontWeight: 500 } },
    }),
    ActionIcon: ActionIcon.extend({
      defaultProps: { variant: 'subtle', color: 'gray' },
    }),
    NavLink: NavLink.extend({
      defaultProps: { variant: 'subtle' },
      styles: {
        root: { borderRadius: 'var(--mantine-radius-md)', paddingBlock: 6 },
        label: { fontSize: 'var(--mantine-font-size-sm)', fontWeight: 500 },
      },
    }),
    Table: Table.extend({
      defaultProps: { verticalSpacing: 'sm', horizontalSpacing: 'lg', highlightOnHoverColor: 'var(--app-hover)' },
      styles: {
        th: {
          fontSize: 'var(--mantine-font-size-xs)',
          fontWeight: 500,
          color: 'var(--mantine-color-dimmed)',
          whiteSpace: 'nowrap',
        },
        td: { borderColor: 'var(--app-border)' },
        tr: { borderColor: 'var(--app-border)' },
      },
    }),
    Tabs: Tabs.extend({
      styles: { tab: { fontWeight: 500, fontSize: 'var(--mantine-font-size-sm)' } },
    }),
    SegmentedControl: SegmentedControl.extend({
      defaultProps: { radius: 'md' },
    }),
    Progress: Progress.extend({
      defaultProps: { radius: 'xl' },
    }),
    Tooltip: Tooltip.extend({
      defaultProps: { openDelay: 200, withArrow: true },
    }),
    Modal: Modal.extend({
      defaultProps: { radius: 'lg', overlayProps: { backgroundOpacity: 0.55, blur: 2 } },
    }),
  },
});

// App-level surface tokens on top of Mantine's, so pages, cards and tables layer the same way in both schemes.
export const resolver: CSSVariablesResolver = () => ({
  variables: {},
  light: {
    '--mantine-color-body': '#ffffff',
    '--app-page': '#fafafa',
    '--app-surface': '#ffffff',
    '--app-subtle': '#f6f6f7',
    '--app-border': '#e8e8eb',
    '--app-hover': '#f6f6f7',
    '--app-active': '#efeff1',
  },
  dark: {
    '--mantine-color-body': '#0c0c0e',
    '--app-page': '#0c0c0e',
    '--app-surface': '#121214',
    '--app-subtle': '#17171a',
    '--app-border': '#232327',
    '--app-hover': '#18181b',
    '--app-active': '#1d1d21',
    '--mantine-color-default-border': '#2a2a2f',
    '--mantine-color-dimmed': '#8a8a93',
  },
});
