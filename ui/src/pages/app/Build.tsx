import { useState, type ReactNode } from 'react';
import { Code, Group, SegmentedControl, SimpleGrid, Stack, Text, Textarea, TextInput } from '@mantine/core';
import { modals } from '@mantine/modals';
import { useQuery } from '@tanstack/react-query';

import { api } from '../../api';
import { fileName, mono, Origin, scriptText } from '../../components/Origin';
import type { App, BuildConfig, Overrides, Runtime, ServerInfo } from '../../types';
import { Section, useSave } from './Settings';

type ScriptKey = 'pre' | 'run' | 'post';

// Build edits how the app is built and started. Every value can come from moonshed.yaml
// or be changed here; changes apply on the next deploy.
export function Build({ app }: { app: App }) {
  const server = useQuery({ queryKey: ['server'], queryFn: () => api.get<ServerInfo>('/server') });
  const windows = server.data?.stats?.os === 'windows';
  const noDocker = server.data !== undefined && !server.data.docker;
  const save = useSave(app);
  const [runtime, setRuntime] = useState<Runtime>(app.runtime);
  const [draft, setDraft] = useState<Overrides>(app.config ?? {});
  const file = app.file;
  const deployed = !!app.current_release_id;

  const patch = (p: Partial<Overrides>) => setDraft((d) => ({ ...d, ...p }));
  const reset = (key: keyof Overrides) =>
    setDraft((d) => {
      const next = { ...d };
      delete next[key];
      return next;
    });
  const changed = runtime !== app.runtime || JSON.stringify(draft) !== JSON.stringify(app.config ?? {});

  const submit = () => {
    const run = () => save.mutate({ runtime, config: draft });
    if (deployed && runtime !== app.runtime) {
      modals.openConfirmModal({
        title: `Switch to ${runtime === 'docker' ? 'Docker' : 'Native Process'}?`,
        children: <Text size="sm">The app stops now. Deploy again to start it with the new runtime.</Text>,
        labels: { confirm: 'Switch and stop', cancel: 'Cancel' },
        confirmProps: { color: 'red' },
        onConfirm: run,
      });
    } else {
      run();
    }
  };

  const script = (key: ScriptKey) => ({
    value: draft[key] ?? scriptText(file?.[key], windows),
    onChange: (v: string) => patch({ [key]: v }),
    origin: <Origin changed={draft[key] !== undefined} inFile={!!scriptText(file?.[key], windows)} onReset={() => reset(key)} />,
  });
  const persist = draft.persist ?? file?.persist ?? [];
  const build: BuildConfig = draft.build ?? file?.build ?? {};
  const setBuild = (p: Partial<BuildConfig>) => patch({ build: { ...build, ...p } });
  // Kept as typed so a half-written line isn't rewritten while typing.
  const [argsText, setArgsText] = useState(() =>
    Object.entries(build.args ?? {})
      .map(([k, v]) => `${k}=${v}`)
      .join('\n'),
  );

  return (
    <Section title="Build" saveHint="Applies on the next deploy" onSave={changed ? submit : undefined} saving={save.isPending}>
      <Stack gap={6}>
        <Text size="sm" fw={500}>
          Runtime
        </Text>
        <SegmentedControl
          w="fit-content"
          value={runtime}
          onChange={(v) => setRuntime(v as Runtime)}
          data={[
            ...(deployed ? [] : [{ value: '', label: 'Detect' }]),
            { value: 'native', label: 'Native Process' },
            { value: 'docker', label: 'Docker', disabled: noDocker && runtime !== 'docker' },
          ]}
        />
        {runtime === '' && (
          <Text size="xs" c="dimmed">
            {noDocker
              ? "Runs as a Native Process. Docker isn't installed or running on this server."
              : 'A run command means Native Process, otherwise a Dockerfile means Docker.'}
          </Text>
        )}
        {runtime !== '' && noDocker && (
          <Text size="xs" c={runtime === 'docker' ? 'red' : 'dimmed'}>
            {runtime === 'docker'
              ? "Docker isn't installed or running on this server, so this app can't build or start. Start Docker, or switch to Native Process."
              : "Docker isn't installed or running on this server."}
          </Text>
        )}
      </Stack>

      {runtime !== 'docker' && (
        <>
          <Field label="Run command" origin={script('run').origin}>
            <Textarea placeholder="python app.py" autosize minRows={2} styles={mono} {...value(script('run'))} />
          </Field>
          <Field label="Before start (optional)" hint="Installs dependencies, runs migrations. Runs on every deploy." origin={script('pre').origin}>
            <Textarea placeholder="pip install -r requirements.txt" autosize minRows={2} styles={mono} {...value(script('pre'))} />
          </Field>
          <Field label="After deploy (optional)" origin={script('post').origin}>
            <Textarea placeholder={'echo "deployed $RELEASE_ID"'} autosize minRows={2} styles={mono} {...value(script('post'))} />
          </Field>
          <Field
            label="Persisted paths (optional)"
            hint="Kept across deploys, one per line. End folders with /"
            origin={<Origin changed={draft.persist !== undefined} inFile={!!file?.persist?.length} onReset={() => reset('persist')} />}
          >
            <Textarea
              placeholder={'data/\nconfig.ini'}
              autosize
              minRows={2}
              styles={mono}
              value={persist.join('\n')}
              onChange={(e) => patch({ persist: e.currentTarget.value.split('\n') })}
              onBlur={() => patch({ persist: persist.map((p) => p.trim()).filter(Boolean) })}
            />
          </Field>
        </>
      )}

      {runtime === 'docker' && (
        <>
          <Group justify="space-between">
            <Text size="sm" c="dimmed">
              Paths are from the repository root.
            </Text>
            <Origin changed={draft.build !== undefined} inFile={!!file?.build} onReset={() => reset('build')} />
          </Group>
          <SimpleGrid cols={{ base: 1, sm: 2 }}>
            <TextInput
              label="Dockerfile"
              placeholder={`${build.folder ? `${build.folder}/` : ''}Dockerfile`}
              styles={mono}
              value={build.dockerfile ?? ''}
              onChange={(e) => setBuild({ dockerfile: e.currentTarget.value })}
            />
            <TextInput
              label="Folder (optional)"
              description="What the Dockerfile can COPY from"
              placeholder="/"
              styles={mono}
              value={build.folder ?? ''}
              onChange={(e) => setBuild({ folder: e.currentTarget.value })}
            />
            <TextInput
              label="Stage (optional)"
              description="Which stage of a multi-stage Dockerfile to build"
              placeholder="runtime"
              styles={mono}
              value={build.stage ?? ''}
              onChange={(e) => setBuild({ stage: e.currentTarget.value })}
            />
            <Textarea
              label="Build arguments (optional)"
              description={
                <>
                  One <Code>KEY=value</Code> per line, for <Code>ARG</Code> in the Dockerfile
                </>
              }
              autosize
              minRows={1}
              styles={mono}
              value={argsText}
              onChange={(e) => {
                setArgsText(e.currentTarget.value);
                setBuild({ args: parseArgs(e.currentTarget.value) });
              }}
            />
          </SimpleGrid>
        </>
      )}
      {!file && deployed && (
        <Text size="xs" c="dimmed">
          The running version has no {fileName}.
        </Text>
      )}
    </Section>
  );
}

function value(s: { value: string; onChange: (v: string) => void }) {
  return { value: s.value, onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => s.onChange(e.currentTarget.value) };
}

function Field({ label, hint, origin, children }: { label: string; hint?: string; origin: ReactNode; children: ReactNode }) {
  return (
    <Stack gap={4}>
      <Group justify="space-between" wrap="nowrap">
        <Text size="sm" fw={500}>
          {label}
        </Text>
        {origin}
      </Group>
      {hint && (
        <Text size="xs" c="dimmed">
          {hint}
        </Text>
      )}
      {children}
    </Stack>
  );
}

function parseArgs(text: string): Record<string, string> {
  const args: Record<string, string> = {};
  for (const line of text.split('\n')) {
    const i = line.indexOf('=');
    if (i > 0) args[line.slice(0, i).trim()] = line.slice(i + 1);
    else if (line.trim()) args[line.trim()] = '';
  }
  return args;
}
