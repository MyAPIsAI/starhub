import ts from "typescript";

// Protocol names, units, brands and identifiers retain their literal spelling.
const technicalText = new Set([
  "myapis.ai",
  "MyAPIs",
  "by Shine AI",
  "HTTP",
  "USD",
  "UTC",
  "OpenAI",
  "Anthropic",
  "Gemini",
  "JSON",
  "Trace ID",
  "cURL",
  "Python",
  "PowerShell",
  "JavaScript",
  "Key",
  "Claude",
  "AWS Bedrock",
  "OpenAI Python",
  "Gemini CLI",
  "Base URL",
  "TTFT P95",
  "ms",
  "(ms)",
  "2FA",
  "⌘ K",
  "P50",
  "P95",
  "(ID",
  "/ v",
  "Terminal",
  "Cursor Settings → Models",
  "Gemini REST",
]);
const nonLatin = /[\p{Script=Han}\p{Script=Arabic}]/u;
const readable = (value) => /[\p{L}]/u.test(value);
const technical = (value) => {
  const text = value.replace(/^[\s·:]+|[\s:·]+$/g, "");
  return (
    technicalText.has(text) ||
    /^[A-Za-z]$/.test(text) ||
    /^\/(?:v1|v1beta)\//.test(text) ||
    /^(GET|POST|PUT|DELETE) \/v1\//.test(text) ||
    /^https?:\/\//.test(text) ||
    /^[\w-]+\.(py|toml)$/.test(text) ||
    /^[a-z]+(?:-[a-z\d.]+)+(?:,[a-z\d.-]+)*$/.test(text)
  );
};

export function findUntranslatedUI(source, relative) {
  const ast = ts.createSourceFile(
    relative,
    source,
    ts.ScriptTarget.Latest,
    true,
    relative.endsWith("tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS,
  );
  const errors = [];
  const fail = (node) =>
    errors.push(
      `${relative}:${ast.getLineAndCharacterOfPosition(node.getStart(ast)).line + 1}: UI text must reference a translation key`,
    );
  const dataLiteral = (node) => {
    // These comparisons identify stored historical values, not displayed copy.
    if (
      ts.isBinaryExpression(node.parent) &&
      [
        ts.SyntaxKind.EqualsEqualsEqualsToken,
        ts.SyntaxKind.ExclamationEqualsEqualsToken,
      ].includes(node.parent.operatorToken.kind)
    )
      return true;
    for (let parent = node.parent; parent; parent = parent.parent) {
      // Search aliases are multilingual index data and must match every locale.
      if (
        relative.replaceAll("\\", "/") === "pages/docs/index.tsx" &&
        ts.isVariableDeclaration(parent) &&
        parent.name.getText(ast) === "articleKeywords"
      )
        return true;
    }
    // The design system intentionally demonstrates an Arabic glyph specimen.
    return (
      relative.replaceAll("\\", "/") === "pages/design-system.tsx" &&
      node.text === "أب"
    );
  };
  const visit = (node) => {
    if (ts.isJsxText(node)) {
      const value = node.text.trim();
      if (readable(value) && !technical(value) && !dataLiteral(node))
        fail(node);
    }
    if (
      ts.isStringLiteral(node) ||
      ts.isNoSubstitutionTemplateLiteral(node) ||
      ts.isTemplateHead(node) ||
      ts.isTemplateMiddle(node) ||
      ts.isTemplateTail(node)
    ) {
      if (
        relative.endsWith("tsx") &&
        nonLatin.test(node.text) &&
        !dataLiteral(node)
      )
        fail(node);
      if (
        ts.isJsxAttribute(node.parent) &&
        ["aria-label", "title", "placeholder", "alt"].includes(
          node.parent.name.getText(ast),
        ) &&
        readable(node.text) &&
        !technical(node.text)
      )
        fail(node);
    }
    // Catch translation dictionaries/tuples outside JSX, including table labels.
    if (ts.isObjectLiteralExpression(node)) {
      const variants = ["zh", "en", "ar"].map((language) =>
        node.properties.find(
          (p) => ts.isPropertyAssignment(p) && p.name.getText(ast) === language,
        ),
      );
      if (
        variants.every(Boolean) &&
        variants.some((p) => nonLatin.test(p.initializer.getText(ast)))
      )
        fail(node);
    }
    if (
      ts.isArrayLiteralExpression(node) &&
      [2, 3].includes(node.elements.length) &&
      node.elements.every(ts.isStringLiteral) &&
      nonLatin.test(node.elements[0].text) &&
      /[A-Za-z]/.test(node.elements[1].text)
    )
      fail(node);
    ts.forEachChild(node, visit);
  };
  visit(ast);
  return [...new Set(errors)];
}
