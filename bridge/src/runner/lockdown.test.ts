import assert from 'node:assert/strict';
import { test } from 'node:test';
import { sessionOptions } from './lockdown.ts';

test('a tool batch ends the turn only when endTurn says so', async () => {
  let end = false;
  const opts = sessionOptions({
    workDir: '/w', srcDir: '/s', configDir: '/c', sessionId: 'id', resume: false, systemPrompt: '',
    mcpServers: {}, mcpTools: [], env: {}, maxTurns: 1, maxBudgetUsd: 1, endTurn: () => end,
  });
  const [stop] = opts.hooks!.PostToolBatch![0].hooks;
  const input = { hook_event_name: 'PostToolBatch', tool_calls: [] } as never;
  const signal = new AbortController().signal;
  assert.deepEqual(await stop(input, undefined, { signal }), {});
  end = true;
  assert.deepEqual(await stop(input, undefined, { signal }), { continue: false, stopReason: 'answered' });
});
