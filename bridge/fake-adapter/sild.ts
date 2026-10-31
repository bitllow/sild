// Stands in for Sild in tests: the key an adapter trusts, and the bridge's assert that signs with it.
import { generateSildTestKey, requesterClaimsFor, signRequester, type SildTrust } from '../src/adapter/kit/index.ts';

const key = generateSildTestKey('test');

export const TEST_TENANT = 't_test';
export const testSild: SildTrust = { tenant: TEST_TENANT, keys: [key.jwk] };
export const testAssert = async (_conv: string, body: string): Promise<string> =>
  signRequester(key.privateKey, 'test', requesterClaimsFor(TEST_TENANT, body));
