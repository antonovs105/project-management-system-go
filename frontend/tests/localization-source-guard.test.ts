import fs from "node:fs";
import path from "node:path";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const sourceRoot = path.resolve("src");
const guardedAttributes = new Set(["label", "title", "placeholder", "aria-label"]);
const allowedTechnicalText = /^(Progo|ActivityPub|GitHub|CSV|JSON|ID|HTTP|HTTPS)$/;

function sourceFiles(directory: string): string[] {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) return entry.name === "generated" ? [] : sourceFiles(target);
    return /\.tsx$/.test(entry.name) ? [target] : [];
  });
}

function requiresTranslation(value: string): boolean {
  const text = value.replace(/\s+/g, " ").trim();
  return /[A-Za-z]{2}/.test(text)
    && !allowedTechnicalText.test(text)
    && !/^https?:\/\//.test(text)
    && !/^[^ ]+@example\.com$/.test(text)
    && text !== "owner/repository"
    && text !== "KiB /";
}

describe("localization source guard", () => {
  it("keeps user-facing source text behind the typed catalog", () => {
    const findings: string[] = [];
    for (const file of sourceFiles(sourceRoot)) {
      const source = ts.createSourceFile(file, fs.readFileSync(file, "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
      const report = (node: ts.Node, text: string) => {
        if (!requiresTranslation(text)) return;
        const line = source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
        findings.push(`${path.relative(sourceRoot, file)}:${line}: ${text.replace(/\s+/g, " ").trim()}`);
      };
      const visit = (node: ts.Node) => {
        if (ts.isJsxText(node)) report(node, node.text);
        if (ts.isJsxAttribute(node) && guardedAttributes.has(node.name.getText(source)) && node.initializer && ts.isStringLiteral(node.initializer)) {
          report(node, node.initializer.text);
        }
        if (ts.isCallExpression(node)) {
          const callee = node.expression.getText(source);
          const candidate = callee === "window.confirm" || callee === "toast.success" || callee === "toast.error"
            ? node.arguments[0]
            : callee === "errorMessage" ? node.arguments.at(-1) : undefined;
          if (candidate && ts.isStringLiteral(candidate)) report(candidate, candidate.text);
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
    expect(findings, findings.join("\n")).toEqual([]);
  });
});
