import { readFileSync } from "node:fs";
import assert from "node:assert/strict";
const read = (language) =>
  JSON.parse(
    readFileSync(new URL(`../src/i18n/${language}.json`, import.meta.url)),
  );
const en = read("en");
for (const language of ["zh", "ar"]) {
  const target = read(language);
  assert.deepEqual(
    Object.keys(target).sort(),
    Object.keys(en).sort(),
    `${language}: translation keys`,
  );
  for (const key of Object.keys(en)) {
    assert.ok(target[key].trim(), `${language}.${key} empty`);
    assert.deepEqual(
      (target[key].match(/\{\{\w+\}\}/g) ?? []).sort(),
      (en[key].match(/\{\{\w+\}\}/g) ?? []).sort(),
      `${language}.${key}: interpolation variables`,
    );
  }
}
console.log(
  `Validated ${Object.keys(en).length} keys across English, Chinese and Arabic.`,
);
