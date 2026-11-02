import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';

const html = fs.readFileSync(new URL('../index.html', import.meta.url), 'utf8');

const inlineScripts = [...html.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/gi)];
assert.ok(inlineScripts.length > 0, 'no inline scripts found');
for (const [index, match] of inlineScripts.entries()) {
  new vm.Script(match[1], {filename: `index.html:inline-${index + 1}`});
}

function extractFunction(name) {
  const patterns = [`function ${name}(`, `async function ${name}(`];
  const start = patterns.map(pattern => html.indexOf(pattern)).find(index => index >= 0);
  assert.notEqual(start, undefined, `missing function ${name}`);
  const bodyStart = html.indexOf('{', start);
  let depth = 0;
  let quote = '';
  let escaped = false;
  for (let i = bodyStart; i < html.length; i++) {
    const char = html[i];
    if (quote) {
      if (escaped) escaped = false;
      else if (char === '\\') escaped = true;
      else if (char === quote) quote = '';
      continue;
    }
    if (char === "'" || char === '"' || char === '`') {
      quote = char;
      continue;
    }
    if (char === '{') depth++;
    if (char === '}') {
      depth--;
      if (depth === 0) return html.slice(start, i + 1);
    }
  }
  throw new Error(`unterminated function ${name}`);
}

const helperNames = [
  'detectImageMimeFromBytes',
  'detectImageMimeFromBase64',
  'normalizeImageMime',
  'imageExtensionForMime',
  'resolveResultImageInfo',
  'createResultImageDataUrl',
  'parseTryonResponse',
  'getErrorMessageFromResponse',
];

const translations = {
  aiFailed: 'failed',
  aiGatewayErr: 'gateway',
  aiInvalidResponse: 'invalid response',
  aiHttpErr: 'http error',
  errAuthRequired: 'auth required',
  errAuthInvalid: 'auth invalid',
  errOriginNotAllowed: 'origin denied',
  errRequestTooLarge: 'too large',
  errInvalidImageBase64: 'invalid image',
  errUnsupportedImageType: 'unsupported image',
  errImageFileTooLarge: 'image too large',
  errImageDimensionsInvalid: 'bad dimensions',
  errImagePixelLimitExceeded: 'too many pixels',
  errRateLimited: 'rate limited',
  errDailyQuotaExceeded: 'quota exceeded',
  errConcurrencyLimited: 'concurrency limited',
  settingsBackendOutdated: 'backend outdated',
};

const context = {
  atob,
  Uint8Array,
  tryonSettings: {useCustom: false},
  t: key => translations[key] || key,
};
vm.createContext(context);
vm.runInContext(helperNames.map(extractFunction).join('\n'), context);

const png = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB';
const jpeg = '/9j/4AAQSkZJRgABAQAAAQABAAD';
const webp = 'UklGRgQAAABXRUJQ';

assert.equal(context.detectImageMimeFromBase64(png), 'image/png');
assert.equal(context.detectImageMimeFromBase64(jpeg), 'image/jpeg');
assert.equal(context.detectImageMimeFromBase64(webp), 'image/webp');
assert.equal(context.imageExtensionForMime('image/jpeg'), 'jpg');
assert.equal(context.imageExtensionForMime('image/webp'), 'webp');

const resolved = context.resolveResultImageInfo(jpeg, 'image/png');
assert.equal(resolved.mime, 'image/jpeg', 'magic bytes must win over a mismatched declaration');
assert.equal(resolved.extension, 'jpg');
assert.match(context.createResultImageDataUrl(resolved), /^data:image\/jpeg;base64,/);

assert.throws(() => context.resolveResultImageInfo('cGxhaW4gdGV4dA==', ''), /unsupported/i);

function fakeResponse({status, contentType, jsonValue, jsonError}) {
  return {
    status,
    ok: status >= 200 && status < 300,
    headers: {get: name => name.toLowerCase() === 'content-type' ? contentType : null},
    json: async () => {
      if (jsonError) throw jsonError;
      return jsonValue;
    },
  };
}

assert.equal(await context.parseTryonResponse(fakeResponse({status: 502, contentType: 'text/html'})), null);
assert.equal(await context.parseTryonResponse(fakeResponse({status: 200, contentType: 'application/json', jsonError: new Error('bad json')})), null);
assert.deepEqual(
  await context.parseTryonResponse(fakeResponse({status: 400, contentType: 'application/problem+json', jsonValue: {message: 'bad'}})),
  {message: 'bad'},
);

assert.equal(context.getErrorMessageFromResponse({status: 413, ok: false}, null).code, 'REQUEST_TOO_LARGE');
assert.equal(context.getErrorMessageFromResponse({status: 401, ok: false}, null).code, 'AUTH_REQUIRED');
assert.equal(context.getErrorMessageFromResponse({status: 403, ok: false}, null).code, 'ORIGIN_NOT_ALLOWED');
assert.match(context.getErrorMessageFromResponse({status: 502, ok: false}, null).text, /502/);
assert.match(context.getErrorMessageFromResponse({status: 200, ok: true}, null).text, /invalid response/);

console.log('frontend try-on tests passed');
