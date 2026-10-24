import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { Ops } from '../slots/types.ts';
import { OPS_TOOLS, opsTools } from './tools.ts';

test('each ops tool calls its op and returns the op result as text', async () => {
  const calls: unknown[] = [];
  const ops: Ops = {
    reply: async (t) => (calls.push(['reply', t]), 'sent'),
    proposeFix: async (c, f) => (calls.push(['fix', c, f]), 'sent'),
    requestInfo: async (n, w) => (calls.push(['info', n, w]), 'requested r1'),
  };
  const [reply, ask, fix, info] = opsTools(ops);
  assert.deepEqual((await reply.handler({ text: 'hi' }, {})).content, [{ type: 'text', text: 'sent' }]);
  await ask.handler({ text: 'which?' }, {});
  await fix.handler({ cause: 'c', fix: ['a', 'b'] }, {});
  assert.deepEqual((await info.handler({ need: 'n', why: 'w' }, {})).content, [{ type: 'text', text: 'requested r1' }]);
  assert.deepEqual(calls, [['reply', 'hi'], ['reply', 'which?'], ['fix', 'c', ['a', 'b']], ['info', 'n', 'w']]);
  assert.deepEqual(OPS_TOOLS, ['mcp__ops__reply', 'mcp__ops__ask_for_details', 'mcp__ops__propose_fix', 'mcp__ops__request_info']);
});

test('a failing op is reported to the model as a tool error', async () => {
  const fail = async (): Promise<string> => {
    throw new Error('sild down');
  };
  const [reply] = opsTools({ reply: fail, proposeFix: fail, requestInfo: fail });
  const res = await reply.handler({ text: 'hi' }, {});
  assert.equal(res.isError, true);
  assert.deepEqual(res.content, [{ type: 'text', text: 'failed: sild down' }]);
});

test('answer tools report a delivered answer; request_info and failures do not', async () => {
  let answered = 0;
  const ok = async () => 'sent';
  const [reply, ask, fix, info] = opsTools({ reply: ok, proposeFix: ok, requestInfo: ok }, () => answered++);
  await reply.handler({ text: 'hi' }, {});
  await ask.handler({ text: 'which?' }, {});
  await fix.handler({ cause: 'c', fix: ['a'] }, {});
  await info.handler({ need: 'n', why: 'w' }, {});
  assert.equal(answered, 3);

  const fail = async (): Promise<string> => {
    throw new Error('sild down');
  };
  const [failing] = opsTools({ reply: fail, proposeFix: fail, requestInfo: fail }, () => answered++);
  await failing.handler({ text: 'hi' }, {});
  assert.equal(answered, 3);
});
