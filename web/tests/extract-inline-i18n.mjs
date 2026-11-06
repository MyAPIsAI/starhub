import ts from "typescript";
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";

const catalogs = Object.fromEntries(
  ["en", "zh", "ar"].map((lang) => {
    const file = `src/i18n/messages.${lang}.json`;
    return [
      lang,
      fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : {},
    ];
  }),
);
let changed = 0;
const skipped = [];
for (const relative of fs.readdirSync("src", { recursive: true })) {
  const file = path.join("src", relative);
  if (
    !/\.(ts|tsx)$/.test(file) ||
    /(?:\.test\.|i18n[\\/])/.test(file) ||
    !fs.statSync(file).isFile()
  )
    continue;
  const source = fs.readFileSync(file, "utf8");
  const ast = ts.createSourceFile(
    file,
    source,
    ts.ScriptTarget.Latest,
    true,
    file.endsWith("tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const edits = [],
    imports = new Set();
  const literal = (n) =>
    ts.isStringLiteral(n) ||
    ts.isNoSubstitutionTemplateLiteral(n) ||
    ts.isTemplateExpression(n);
  const visit = (node) => {
    if (
      ts.isConditionalExpression(node) &&
      ts.isBinaryExpression(node.condition) &&
      node.condition.right.getText(ast) === '"zh"' &&
      ts.isStringLiteral(node.whenTrue) &&
      /\p{Script=Han}/u.test(node.whenTrue.text) &&
      ts.isConditionalExpression(node.whenFalse)
    ) {
      const rest = node.whenFalse;
      if (
        ts.isBinaryExpression(rest.condition) &&
        rest.condition.left.getText(ast) === node.condition.left.getText(ast) &&
        rest.condition.right.getText(ast) === '"ar"' &&
        ts.isStringLiteral(rest.whenTrue) &&
        ts.isStringLiteral(rest.whenFalse)
      ) {
        const zh = node.whenTrue.text,
          ar = rest.whenTrue.text,
          en = rest.whenFalse.text;
        const key =
          en
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, "_")
            .replace(/^_|_$/g, "")
            .slice(0, 45) +
          "_" +
          crypto
            .createHash("sha256")
            .update(JSON.stringify([en, zh, ar]))
            .digest("hex")
            .slice(0, 10);
        catalogs.en[key] = en;
        catalogs.zh[key] = zh;
        catalogs.ar[key] = ar;
        imports.add("localizedMessage");
        edits.push([
          node.getStart(ast),
          node.getEnd(),
          "localizedMessage(" +
            JSON.stringify(key) +
            ", " +
            node.condition.left.getText(ast) +
            ")",
        ]);
        return;
      }
    }

    if (
      ts.isArrayLiteralExpression(node) &&
      [2, 3].includes(node.elements.length) &&
      node.elements.every(
        (n) => ts.isStringLiteral(n) || ts.isNoSubstitutionTemplateLiteral(n),
      ) &&
      /\p{Script=Han}/u.test(node.elements[0].text) &&
      !/\p{Script=Han}/u.test(node.elements[1].text)
    ) {
      const [zh, en, ar = en] = node.elements.map((n) => n.text);
      if (node.elements.length === 3 && !/\p{Script=Arabic}/u.test(ar)) {
        ts.forEachChild(node, visit);
        return;
      }
      const key =
        en
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, "_")
          .replace(/^_|_$/g, "")
          .slice(0, 45) +
        "_" +
        crypto
          .createHash("sha256")
          .update(JSON.stringify([en, zh, ar]))
          .digest("hex")
          .slice(0, 10);
      catalogs.en[key] = en;
      catalogs.zh[key] = zh;
      catalogs.ar[key] = ar;
      const helper =
        node.elements.length === 3 ? "translatedMessage" : "messagePair";
      imports.add(helper);
      edits.push([
        node.getStart(ast),
        node.getEnd(),
        `${helper}(${JSON.stringify(key)})`,
      ]);
      return;
    }
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      ["t", "text", "c"].includes(node.expression.text) &&
      [2, 3].includes(node.arguments.length) &&
      node.arguments.every(literal)
    ) {
      const expressions = [];
      const parse = (n) => {
        if (!ts.isTemplateExpression(n)) return n.text;
        let value = n.head.text;
        for (const span of n.templateSpans) {
          const expression = span.expression.getText(ast);
          let index = expressions.indexOf(expression);
          if (index < 0) {
            index = expressions.length;
            expressions.push(expression);
          }
          value += `{{p${index}}}` + span.literal.text;
        }
        return value;
      };
      const en = parse(node.arguments[1]),
        zh = parse(node.arguments[0]);
      const ar = node.arguments.length === 3 ? parse(node.arguments[2]) : en;
      const params = (v) =>
        [...new Set(v.match(/\{\{p\d+\}\}/g) ?? [])].sort().join();
      if (params(en) !== params(zh) || params(en) !== params(ar)) {
        skipped.push(
          `${file}:${ast.getLineAndCharacterOfPosition(node.getStart()).line + 1}`,
        );
        return;
      }
      const key =
        en
          .toLowerCase()
          .replace(/\{\{.*?\}\}/g, "")
          .replace(/[^a-z0-9]+/g, "_")
          .replace(/^_|_$/g, "")
          .slice(0, 45) +
        "_" +
        crypto
          .createHash("sha256")
          .update(JSON.stringify([en, zh, ar]))
          .digest("hex")
          .slice(0, 10);
      catalogs.en[key] = en;
      catalogs.zh[key] = zh;
      catalogs.ar[key] = ar;
      const helper =
        node.arguments.length === 3 ? "translatedMessage" : "messagePair";
      imports.add(helper);
      const values = expressions.length
        ? `, { ${expressions.map((v, i) => `p${i}: (${v})`).join(", ")} }`
        : "";
      edits.push([
        node.arguments[0].getStart(ast),
        node.arguments.at(-1).getEnd(),
        `...${helper}(${JSON.stringify(key)}${values})`,
      ]);
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(ast);
  if (!edits.length) continue;
  for (const statement of ast.statements) {
    if (
      ts.isImportDeclaration(statement) &&
      statement.moduleSpecifier.text.endsWith("i18n/messages")
    ) {
      for (const binding of statement.importClause.namedBindings.elements)
        imports.add(binding.name.text);
      edits.push([statement.getStart(ast), statement.getEnd(), ""]);
    }
  }
  let result = source;
  for (const [start, end, replacement] of edits.sort((a, b) => b[0] - a[0]))
    result = result.slice(0, start) + replacement + result.slice(end);
  const module = path
    .relative(path.dirname(file), "src/i18n/messages")
    .replaceAll("\\", "/");
  result =
    `import { ${[...imports].map((name) => (name === "translatedMessage" ? "message as translatedMessage" : name)).join(", ")} } from ${JSON.stringify(module.startsWith(".") ? module : "./" + module)};\n` +
    result;
  fs.writeFileSync(file, result);
  changed += edits.length;
}
for (const lang of ["en", "zh", "ar"])
  fs.writeFileSync(
    `src/i18n/messages.${lang}.json`,
    JSON.stringify(catalogs[lang], null, 2) + "\n",
  );
console.log(
  JSON.stringify({
    extracted: changed,
    keys: Object.keys(catalogs.en).length,
    skipped,
  }),
);
