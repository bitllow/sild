import { describe, test } from 'node:test';
import assert from 'node:assert/strict';
import { isBlockedAddress, resolveTarget, type Resolver } from './net.ts';

const dns = (address: string, family = 4): Resolver => async () => [{ address, family }];

describe('isBlockedAddress', () => {
  for (const a of [
    '10.1.2.3', '172.16.0.1', '172.31.255.255', '192.168.1.1', '127.0.0.1', '169.254.169.254', '100.64.0.1', '0.0.0.0',
    '224.0.0.1', '255.255.255.255', '::1', '::', 'fe80::1', 'fc00::1', 'fd12:3456::1', '::ffff:10.0.0.1', '::ffff:127.0.0.1',
    '64:ff9b::a00:1', '2002:a00:1::1', 'ff02::1', 'not-an-ip',
  ]) {
    test(`blocks ${a}`, () => assert.equal(isBlockedAddress(a), true));
  }
  for (const a of ['93.184.216.34', '8.8.8.8', '172.32.0.1', '2606:4700::1111', '::ffff:8.8.8.8']) {
    test(`allows ${a}`, () => assert.equal(isBlockedAddress(a), false));
  }
});

describe('resolveTarget', () => {
  test('pins the public address', async () => {
    const t = await resolveTarget(new URL('https://adapter.example.com/x'), false, dns('93.184.216.34'));
    assert.equal(t.address, '93.184.216.34');
  });
  test('refuses http', async () => {
    await assert.rejects(resolveTarget(new URL('http://adapter.example.com/'), false, dns('93.184.216.34')), /https/);
  });
  test('refuses a host that resolves privately', async () => {
    await assert.rejects(resolveTarget(new URL('https://adapter.example.com/'), false, dns('10.0.0.5')), /non-public/);
  });
  test('refuses if any answer is private', async () => {
    const mixed: Resolver = async () => [{ address: '93.184.216.34', family: 4 }, { address: '169.254.169.254', family: 4 }];
    await assert.rejects(resolveTarget(new URL('https://adapter.example.com/'), false, mixed), /non-public/);
  });
  test('refuses a literal private IP', async () => {
    await assert.rejects(resolveTarget(new URL('https://192.168.0.10/'), false, dns('1.1.1.1')), /non-public/);
  });
  test('refuses credentials in the URL', async () => {
    await assert.rejects(resolveTarget(new URL('https://u:p@adapter.example.com/'), false, dns('93.184.216.34')), /credentials/);
  });
  test('refuses localhost without the flag', async () => {
    await assert.rejects(resolveTarget(new URL('http://127.0.0.1:8790/'), false, dns('127.0.0.1')));
  });
  test('allows http localhost with allowInsecureLocal', async () => {
    const t = await resolveTarget(new URL('http://localhost:8790/'), true, dns('127.0.0.1'));
    assert.equal(t.address, '127.0.0.1');
  });
  test('allowInsecureLocal does not open private ranges', async () => {
    await assert.rejects(resolveTarget(new URL('http://localhost:8790/'), true, dns('10.0.0.5')), /non-public/);
    await assert.rejects(resolveTarget(new URL('http://10.0.0.5/'), true, dns('10.0.0.5')), /https/);
  });
});
