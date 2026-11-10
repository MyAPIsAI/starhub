import { readFileSync, readdirSync, statSync } from "node:fs";
import assert from "node:assert/strict";
import path from "node:path";
import ts from "typescript";
import { findUntranslatedUI } from "./i18n-source-check.mjs";
const root = new URL("../", import.meta.url);
const read = (name) =>
  JSON.parse(readFileSync(new URL(`src/i18n/${name}.json`, root), "utf8"));
const namespaces = [
  ["", read("en"), read("zh"), read("ar")],
  ["messages.", read("messages.en"), read("messages.zh"), read("messages.ar")],
];
for (const [prefix, en, ...translations] of namespaces)
  for (const target of [en, ...translations]) {
    assert.deepEqual(
      Object.keys(target).sort(),
      Object.keys(en).sort(),
      `${prefix}translation keys`,
    );
    for (const key of Object.keys(en)) {
      assert.ok(target[key].trim(), `${prefix}${key} empty`);
      assert.deepEqual(
        (target[key].match(/\{\{\w+\}\}/g) ?? []).sort(),
        (en[key].match(/\{\{\w+\}\}/g) ?? []).sort(),
        `${prefix}${key}: placeholders`,
      );
    }
  }
const used = new Set(),
  usedMessages = new Set(),
  errors = [];
const en = namespaces[0][1],
  messages = namespaces[1][1];
const src = new URL("src/", root);
for (const relative of readdirSync(src, { recursive: true })) {
  const url = new URL(relative.replaceAll("\\", "/"), src);
  if (
    !/\.(ts|tsx)$/.test(relative) ||
    /\.test\.|i18n[\\/]/.test(relative) ||
    !statSync(url).isFile()
  )
    continue;
  const source = readFileSync(url, "utf8");
  errors.push(...findUntranslatedUI(source, relative));
  const ast = ts.createSourceFile(
    relative,
    source,
    99,
    true,
    relative.endsWith("tsx") ? 4 : 3,
  );
  const visit = (node) => {
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) {
      if (Object.hasOwn(en, node.text)) used.add(node.text);
    }
    if (ts.isCallExpression(node)) {
      const name = ts.isIdentifier(node.expression)
        ? node.expression.text
        : ts.isPropertyAccessExpression(node.expression)
          ? node.expression.name.text
          : "";
      const arg = node.arguments[0];
      if (
        ["translatedMessage", "messagePair", "localizedMessage"].includes(
          name,
        ) &&
        arg &&
        ts.isStringLiteral(arg)
      ) {
        if (!Object.hasOwn(messages, arg.text))
          errors.push(`${relative}: missing message ${arg.text}`);
        else usedMessages.add(arg.text);
      }
      if (name === "t" && source.includes("useTranslation") && arg) {
        if (
          (ts.isStringLiteral(arg) ||
            ts.isNoSubstitutionTemplateLiteral(arg)) &&
          node.arguments.length < 3 &&
          !Object.hasOwn(en, arg.text)
        )
          errors.push(`${relative}: missing key ${arg.text}`);
        if (ts.isTemplateExpression(arg)) {
          const prefix = arg.head.text,
            suffix = arg.templateSpans.at(-1).literal.text;
          if (prefix || suffix)
            for (const key of Object.keys(en))
              if (key.startsWith(prefix) && key.endsWith(suffix)) used.add(key);
        }
      }
      if (
        ["t", "text", "c"].includes(name) &&
        [2, 3].includes(node.arguments.length) &&
        node.arguments.every(
          (v) =>
            ts.isStringLiteral(v) ||
            ts.isNoSubstitutionTemplateLiteral(v) ||
            ts.isTemplateExpression(v),
        )
      )
        errors.push(`${relative}: inline translations must use catalog keys`);
    }
    ts.forEachChild(node, visit);
  };
  visit(ast);
}
for (const [catalog, uses, prefix] of [
  [en, used, ""],
  [messages, usedMessages, "messages."],
])
  for (const key of Object.keys(catalog))
    if (!uses.has(key)) errors.push(`Unused key: ${prefix}${key}`);
assert.deepEqual(errors, [], errors.join("\n"));
console.log(
  `Validated ${Object.keys(en).length + Object.keys(messages).length} keys: complete en/zh/ar, matching placeholders, usage and no inline translation calls.`,
);
