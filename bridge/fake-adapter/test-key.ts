// Prints a Sild test key for a conformance run: SILD_TEST_KEY for the suite, SILD_KEYS for the adapter.
//   eval "$(node fake-adapter/test-key.ts)"
import { generateSildTestKey } from '../src/adapter/kit/index.ts';

const { privateKey, jwk } = generateSildTestKey('conformance');
console.log(`export SILD_TEST_KEY='${JSON.stringify({ ...privateKey.export({ format: 'jwk' }), kid: jwk.kid })}'`);
console.log(`export SILD_KEYS='${JSON.stringify([jwk])}'`);
