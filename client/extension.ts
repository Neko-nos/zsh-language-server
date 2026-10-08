import * as path from "node:path";
import * as vscode from "vscode";
import {
  LanguageClient,
  LSPErrorCodes,
  ResponseError,
} from "vscode-languageclient/node";

export async function activate(context: vscode.ExtensionContext) {
  const pendingOpens = new Set<Promise<void>>();
  async function detectLanguage(document: vscode.TextDocument) {
    if (
      document.isClosed ||
      !["shellscript", "zsh"].includes(document.languageId)
    )
      return;
    const firstLine = document.lineAt(0).text;
    let language = "zsh";
    if (
      !/^#!\s*(?:\S*\/)?(?:zsh|env\s+(?:-S\s+)?zsh)(?:\s|$)/.test(firstLine)
    ) {
      if (!document.uri.path.endsWith(".sh")) return;
      let shellcheck = false;
      for (let line = 0; line < document.lineCount; line++) {
        const text = document.lineAt(line).text.trim();
        if (text && !text.startsWith("#")) break;
        if (/^#\s*shellcheck(?:\s|$)/.test(text)) {
          shellcheck = true;
          break;
        }
      }
      const fallback = vscode.workspace
        .getConfiguration("zsh", document)
        .get<boolean>("assumeZsh");
      if (shellcheck || firstLine.startsWith("#!") || !fallback)
        language = "shellscript";
    }
    if (document.languageId === language) return;
    const associations = vscode.workspace
      .getConfiguration("files", document)
      .get<Record<string, string>>("associations", {});
    if (
      Object.keys(associations).some((pattern) =>
        vscode.languages.match(
          { pattern: pattern.includes("/") ? pattern : `**/${pattern}` },
          document,
        ),
      )
    )
      return;
    await vscode.languages.setTextDocumentLanguage(document, language);
  }
  context.subscriptions.push(
    vscode.workspace.onDidOpenTextDocument(detectLanguage),
    vscode.workspace.onDidChangeTextDocument(({ document, contentChanges }) => {
      if (contentChanges.length) void detectLanguage(document);
    }),
    vscode.workspace.onDidChangeConfiguration((event) => {
      if (event.affectsConfiguration("zsh.assumeZsh"))
        void Promise.all(vscode.workspace.textDocuments.map(detectLanguage));
    }),
  );
  await Promise.all(vscode.workspace.textDocuments.map(detectLanguage));
  const command = context.asAbsolutePath(
    path.join(
      "bin",
      `${process.platform}-${process.arch}`,
      "zsh-language-server",
    ),
  );
  const client = new LanguageClient(
    "zsh",
    "Zsh Language Server",
    { command },
    {
      middleware: {
        async didOpen(document, next) {
          const pending = next(document);
          pendingOpens.add(pending);
          try {
            await pending;
          } finally {
            pendingOpens.delete(pending);
          }
        },
        async sendRequest(type, params, token, next) {
          // Initial document opens can still be in flight when providers become available.
          while (pendingOpens.size) await Promise.all(pendingOpens);
          // Changing language mode can close a document while its request is waiting.
          const uri = (params as { textDocument?: { uri: string } } | undefined)
            ?.textDocument?.uri;
          if (
            token?.isCancellationRequested ||
            (uri &&
              !vscode.workspace.textDocuments.some(
                (document) =>
                  document.uri.toString() === uri &&
                  !document.isClosed &&
                  document.languageId === "zsh",
              ))
          ) {
            throw new ResponseError(
              LSPErrorCodes.RequestCancelled,
              "Request cancelled",
            );
          }
          return next(type, params, token);
        },
      },
      documentSelector: [
        { language: "zsh", scheme: "file" },
        { language: "zsh", scheme: "untitled" },
      ],
    },
  );
  context.subscriptions.push(client);
  context.subscriptions.push(
    vscode.commands.registerCommand("zsh.restartServer", async () => {
      await client.stop();
      await client.start();
    }),
  );
  await client.start();
}
