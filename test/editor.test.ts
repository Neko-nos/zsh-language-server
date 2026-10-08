import * as assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as vscode from "vscode";

function at(document: vscode.TextDocument, text: string, offset = 0) {
  const index = document.getText().lastIndexOf(text);
  assert.ok(index >= 0, `Missing test text: ${text}`);
  return document.positionAt(index + offset);
}

async function definitions(
  document: vscode.TextDocument,
  text: string,
  offset = 0,
) {
  return (
    (await vscode.commands.executeCommand<vscode.Location[]>(
      "vscode.executeDefinitionProvider",
      document.uri,
      at(document, text, offset),
    )) ?? []
  );
}

async function definitionLine(
  document: vscode.TextDocument,
  text: string,
  line: number,
  offset = 0,
) {
  const locations = await definitions(document, text, offset);
  assert.equal(locations.length, 1, `Definition for ${text}`);
  assert.equal(locations[0].range.start.line, line, `Definition for ${text}`);
}

export async function runEditorTests(directory: string) {
  const failures: Error[] = [];
  let passed = 0;
  async function check(
    name: string,
    text: string,
    inspect: (document: vscode.TextDocument) => Promise<void>,
  ) {
    try {
      const uri = vscode.Uri.file(path.join(directory, `${name}.zsh`));
      await fs.writeFile(uri.fsPath, text);
      const document = await vscode.workspace.openTextDocument(uri);
      await inspect(document);
      console.log(`Editor case passed: ${name}`);
      passed++;
    } catch (error) {
      const failure = new Error(`${name}: ${String(error)}`);
      failures.push(failure);
      console.error(failure.message);
    }
  }

  await check(
    "quoted-command",
    "draw_tile() { :; }\n'draw_tile'\n",
    async (document) => {
      await definitionLine(document, "'draw_tile'", 0, 2);
      const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
        "vscode.executeDocumentRenameProvider",
        document.uri,
        at(document, "'draw_tile'", 2),
        "paint_tile",
      );
      assert.ok(edit);
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(document.getText(), "paint_tile() { :; }\n'paint_tile'\n");
    },
  );
  await check(
    "function-name",
    "draw-tile() { :; }\ndraw-tile\n",
    async (document) => {
      await definitionLine(document, "draw-tile", 0);
      const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
        "vscode.executeDocumentRenameProvider",
        document.uri,
        at(document, "draw-tile"),
        "tile.paint",
      );
      assert.ok(edit);
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(document.getText(), "tile.paint() { :; }\ntile.paint\n");
      await definitionLine(document, "tile.paint", 0);
      const suggestions =
        await vscode.commands.executeCommand<vscode.CompletionList>(
          "vscode.executeCompletionItemProvider",
          document.uri,
          at(document, "tile.paint", 6),
        );
      const item = suggestions?.items.find(
        (item) => item.label === "tile.paint",
      );
      assert.ok(item);
      const range = item.range;
      assert.ok(range instanceof vscode.Range);
      assert.equal(document.getText(range), "tile.paint");
    },
  );

  for (const command of [
    "noglob draw_tile",
    "add-zsh-hook precmd draw_tile",
    "zle -N sample-widget draw_tile",
    "'compdef' draw_tile sample",
    "builtin zle -N sample-widget draw_tile",
  ]) {
    await check(
      `callback-${command.split(" ")[0]}`,
      `draw_tile() { :; }\n${command}\n`,
      async (document) => {
        await definitionLine(document, "draw_tile", 0);
      },
    );
  }

  await check(
    "external-command",
    "draw_tile() { :; }\ncommand draw_tile\n",
    async (document) => {
      assert.equal((await definitions(document, "draw_tile")).length, 0);
    },
  );
  await check(
    "command-prefix-rename",
    "printf() { :; }\nexec -a dummy printf sample\ncommand printf sample\nbuiltin printf sample\n",
    async (document) => {
      const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
        "vscode.executeDocumentRenameProvider",
        document.uri,
        new vscode.Position(1, 16),
        "print_tile",
      );
      assert.ok(edit);
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(
        document.getText(),
        "print_tile() { :; }\nexec -a dummy print_tile sample\ncommand printf sample\nbuiltin printf sample\n",
      );
    },
  );
  await check(
    "implicit-widget-rename",
    "draw_tile() { :; }\nzle -N 'draw_tile'\n",
    async (document) => {
      const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
        "vscode.executeDocumentRenameProvider",
        document.uri,
        new vscode.Position(0, 2),
        "paint_tile",
      );
      assert.ok(edit);
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(
        document.getText(),
        "paint_tile() { :; }\nzle -N 'draw_tile' paint_tile\n",
      );
      await definitionLine(document, "paint_tile", 0);
    },
  );

  await check(
    "parameter-flags",
    `palette=(amber blue)\nprint \${(U)palette}\n`,
    async (document) => {
      await definitionLine(document, "palette", 0);
    },
  );
  await check(
    "indexed-array",
    `typeset -a palette=(amber blue)\nslot=1\nprint \${palette[slot]}\n`,
    async (document) => {
      await definitionLine(document, "palette", 0);
      await definitionLine(document, "slot", 1);
    },
  );
  await check(
    "indexed-array-assignment",
    "typeset -a palette=(amber blue)\nslot=1\npalette[slot]=green\n",
    async (document) => {
      await definitionLine(document, "slot", 1);
      const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
        "vscode.executeDocumentRenameProvider",
        document.uri,
        at(document, "slot"),
        "index",
      );
      assert.ok(edit);
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(
        document.getText(),
        "typeset -a palette=(amber blue)\nindex=1\npalette[index]=green\n",
      );
    },
  );
  await check(
    "associative-assignment-key",
    "typeset -A palette=(shade amber)\nshade=blue\npalette[shade]=green\n",
    async (document) => {
      assert.equal((await definitions(document, "shade")).length, 0);
    },
  );
  await check(
    "associative-key",
    `typeset -A palette=(shade amber)\nshade=blue\nprint \${palette[shade]}\n`,
    async (document) => {
      assert.equal((await definitions(document, "shade")).length, 0);
    },
  );
  await check(
    "associative-expanded-key",
    `typeset -A palette=(shade amber)\nshade=blue\nprint \${palette[$shade]}\n`,
    async (document) => {
      await definitionLine(document, "$shade", 1, 1);
    },
  );
  await check(
    "inspect-function",
    "draw_tile() { :; }\ntypeset -f draw_tile\n",
    async (document) => {
      await definitionLine(document, "draw_tile", 0);
    },
  );
  await check(
    "unset-parameter",
    "shade=amber\nunset -v shade\n",
    async (document) => {
      await definitionLine(document, "shade", 0);
    },
  );
  await check(
    "inspect-parameter",
    "shade=amber\ntypeset -p shade\n",
    async (document) => {
      await definitionLine(document, "shade", 0);
      const symbols = await vscode.commands.executeCommand<
        vscode.DocumentSymbol[]
      >("vscode.executeDocumentSymbolProvider", document.uri);
      assert.equal(
        symbols?.filter((symbol) => symbol.name === "shade").length,
        1,
      );
    },
  );

  await check(
    "local-rename",
    "shade=amber\npaint() {\n local shade=blue\n print $shade\n}\nprint $shade\n",
    async (document) => {
      const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
        "vscode.executeDocumentRenameProvider",
        document.uri,
        new vscode.Position(3, 9),
        "tone",
      );
      assert.ok(edit);
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(
        document.getText(),
        "shade=amber\npaint() {\n local tone=blue\n print $tone\n}\nprint $shade\n",
      );
    },
  );
  await check(
    "rename-capture",
    "shade=amber\npaint() {\n local tone=blue\n print $shade $tone\n}\n",
    async (document) => {
      await assert.rejects(async () =>
        vscode.commands.executeCommand(
          "vscode.executeDocumentRenameProvider",
          document.uri,
          at(document, "$shade", 1),
          "tone",
        ),
      );
    },
  );
  await check(
    "global-declaration",
    "shade=amber\npaint() {\n local shade=blue\n typeset -g shade=green\n print $shade\n}\nprint $shade\n",
    async (document) => {
      const refs = await vscode.commands.executeCommand<vscode.Location[]>(
        "vscode.executeReferenceProvider",
        document.uri,
        new vscode.Position(2, 8),
      );
      assert.deepEqual(
        refs?.map((location) => location.range.start.line),
        [2, 3, 4],
      );
    },
  );

  await check(
    "parameter-completion",
    "palette=amber\nprint $pal-tail\n",
    async (document) => {
      const completions =
        await vscode.commands.executeCommand<vscode.CompletionList>(
          "vscode.executeCompletionItemProvider",
          document.uri,
          at(document, "$pal", 4),
        );
      const item = completions?.items.find((item) => item.label === "palette");
      assert.ok(item, "Missing parameter completion before a hyphen");
      const range = item.range;
      assert.ok(range instanceof vscode.Range);
      assert.equal(document.getText(range), "pal");
      const edit = new vscode.WorkspaceEdit();
      edit.replace(document.uri, range, "palette");
      assert.equal(await vscode.workspace.applyEdit(edit), true);
      assert.equal(document.getText(), "palette=amber\nprint $palette-tail\n");
    },
  );
  for (const [name, body] of [
    ["argument", "print draw_t"],
    ["path", "source ./draw_t"],
    ["heredoc", "cat <<'TEXT'\ndraw_t\nTEXT"],
    ["comment", "# draw_t"],
    ["string", "print 'draw_t'"],
  ]) {
    await check(
      `completion-${name}`,
      `draw_tile() { :; }\n${body}\n`,
      async (document) => {
        const completions =
          await vscode.commands.executeCommand<vscode.CompletionList>(
            "vscode.executeCompletionItemProvider",
            document.uri,
            at(document, "draw_t", 6),
          );
        assert.ok(
          !completions?.items.some((item) => item.label === "draw_tile"),
          "Function suggested in literal text",
        );
      },
    );
  }
  await check(
    "literal-heredoc",
    "shade=amber\ncat <<'TEXT'\n$shade\nTEXT\n",
    async (document) => {
      assert.equal((await definitions(document, "$shade", 1)).length, 0);
    },
  );
  await check(
    "expanded-heredoc",
    "shade=amber\ncat <<TEXT\n$shade\nTEXT\n",
    async (document) => {
      await definitionLine(document, "$shade", 0, 1);
    },
  );

  const library = vscode.Uri.file(
    path.join(directory, "library with spaces.zsh"),
  );
  await fs.writeFile(library.fsPath, "draw_tile() { :; }\n");
  await check(
    "source-variable",
    `library='${library.fsPath}'\nsource "$library"\ndraw_tile\n`,
    async (document) => {
      const locations = await definitions(document, "draw_tile");
      assert.equal(locations[0]?.uri.toString(), library.toString());
      const target = await definitions(document, '"$library"');
      assert.equal(target[0]?.uri.toString(), library.toString());
    },
  );
  await check(
    "source-local-shadow",
    `library='${library.fsPath}'\nload_tiles() {\n local library=$1\n source "$library"\n}\ndraw_tile\n`,
    async (document) => {
      assert.equal((await definitions(document, "draw_tile")).length, 0);
    },
  );
  for (const [name, body] of [
    ["quoted", `'source' '${library.fsPath}'`],
    ["builtin", `builtin source '${library.fsPath}'`],
    ["builtin-dot", `builtin . '${library.fsPath}'`],
    [
      "redeclared-local",
      `load_tiles() { local library='${library.fsPath}'; local library; source "$library"; }`,
    ],
    [
      "local",
      `load_tiles() { local library='${library.fsPath}'; source "$library"; }`,
    ],
    [
      "dot",
      `load_tiles() { local library='${library.fsPath}'; . "$library"; }`,
    ],
    [
      "empty-default",
      `load_tiles() { local library=; source "\${library:-${library.fsPath}}"; }`,
    ],
    [
      "if",
      `if [[ $choice == yes ]]; then library='${library.fsPath}'; source "$library"; fi`,
    ],
    [
      "else",
      `library='${library.fsPath}'\nif [[ $choice == yes ]]; then library=$1; else source "$library"; fi`,
    ],
    [
      "while",
      `while [[ $choice == yes ]]; do library='${library.fsPath}'; source "$library"; done`,
    ],
    [
      "for",
      `for item in one two; do library='${library.fsPath}'; source "$library"; done`,
    ],
    [
      "case",
      `case $choice in yes) library='${library.fsPath}'; source "$library";; esac`,
    ],
    [
      "directive",
      `# shellcheck source="${library.fsPath}"\nsource "$1" || true`,
    ],
    [
      "directory-hint",
      `# shellcheck source-path="${directory}"\nsource 'library with spaces.zsh'`,
    ],
    ["escaped", `source ./library\\ with\\ spaces.zsh`],
    ["ansi-quote", `source $'./library with spaces.zsh'`],
  ]) {
    await check(`source-${name}`, `${body}\ndraw_tile\n`, async (document) => {
      const locations = await definitions(document, "draw_tile");
      assert.equal(locations.length, 1);
      assert.equal(locations[0].uri.toString(), library.toString());
      const hover = await vscode.commands.executeCommand<vscode.Hover[]>(
        "vscode.executeHoverProvider",
        document.uri,
        at(document, "draw_tile"),
      );
      assert.ok(hover?.length);
    });
  }
  const alias = vscode.Uri.file(path.join(directory, "library-link.zsh"));
  await fs.symlink(library.fsPath, alias.fsPath);
  await check(
    "source-shared-closed-symlink",
    "source './library with spaces.zsh'\nsource ./library-link.zsh\ndraw_tile\n",
    async (document) => {
      const locations = await definitions(document, "draw_tile");
      assert.equal(locations.length, 1);
      assert.equal(locations[0].uri.toString(), library.toString());
    },
  );
  const libraryDocument = await vscode.workspace.openTextDocument(library);
  const libraryEdit = new vscode.WorkspaceEdit();
  libraryEdit.insert(library, new vscode.Position(0, 0), "# Unsaved heading\n");
  assert.equal(await vscode.workspace.applyEdit(libraryEdit), true);
  await check(
    "source-unsaved-symlink",
    "source ./library-link.zsh\ndraw_tile\n",
    async (document) => {
      await definitionLine(document, "draw_tile", 1);
      assert.equal(
        (await definitions(document, "draw_tile"))[0].uri.toString(),
        libraryDocument.uri.toString(),
      );
    },
  );
  await check(
    "source-shared-symlink",
    "source ./library-link.zsh\nsource './library with spaces.zsh'\ndraw_tile\n",
    async (document) => {
      await definitionLine(document, "draw_tile", 1);
    },
  );
  const cycle = path.join(directory, "source-self-symlink.zsh");
  await fs.symlink(cycle, path.join(directory, "self-link.zsh"));
  await check(
    "source-self-symlink",
    "draw_tile() { :; }\nsource ./self-link.zsh\ndraw_tile\n",
    async (document) => {
      await definitionLine(document, "draw_tile", 0);
    },
  );
  const outside = path.join(path.dirname(directory), "outside");
  await fs.mkdir(outside);
  await fs.writeFile(path.join(outside, "helper.zsh"), "draw_tile() { :; }\n");
  await fs.writeFile(
    path.join(outside, "main.zsh"),
    "source ./helper.zsh\ndraw_tile\n",
  );
  await check("source-outside-workspace", "", async () => {
    const document = await vscode.workspace.openTextDocument(
      path.join(outside, "main.zsh"),
    );
    assert.equal(
      (await definitions(document, "draw_tile"))[0]?.uri.fsPath,
      path.join(outside, "helper.zsh"),
    );
  });
  await check(
    "source-external-absolute",
    `source '${path.join(outside, "helper.zsh")}'\ndraw_tile\n`,
    async (document) => {
      assert.equal(
        (await definitions(document, "draw_tile"))[0]?.uri.fsPath,
        path.join(outside, "helper.zsh"),
      );
    },
  );
  const firstPath = path.join(directory, "first-autoload");
  const secondPath = path.join(directory, "second-autoload");
  for (const folder of [firstPath, secondPath]) {
    await fs.mkdir(folder);
    await fs.writeFile(
      path.join(folder, "paint_tile"),
      "# Paint a tile.\npaint_tile() { :; }\n",
    );
  }
  const loader = path.join(directory, "loader.zsh");
  await fs.writeFile(
    loader,
    `autoload -Uz "\${\${(%):-%N}:A:h}/first-autoload/paint_tile"\n`,
  );
  await check(
    "source-expanded-autoload",
    `source "\${\${(%):-%N}:A:h}/loader.zsh"\nrender_tile() { paint_tile; }\n`,
    async (document) => {
      const result = await definitions(document, "paint_tile");
      assert.equal(result.length, 1);
      assert.equal(result[0].uri.fsPath, path.join(firstPath, "paint_tile"));
      assert.equal(result[0].range.start.line, 1);
      const hover = await vscode.commands.executeCommand<vscode.Hover[]>(
        "vscode.executeHoverProvider",
        document.uri,
        at(document, "paint_tile"),
      );
      const markdown = hover
        ?.flatMap((item) => item.contents)
        .map((part) => (typeof part === "string" ? part : part.value))
        .join("\n");
      assert.match(markdown ?? "", /Paint a tile/);
    },
  );
  await check(
    "external-command-hover",
    "mkdir -p sample\ncurl https://example.invalid/sample | bash\n",
    async (document) => {
      for (const [command, description] of [
        ["mkdir", /make directories/i],
        ["curl", /transfer/i],
        ["bash", /command language/i],
      ] as const) {
        const hover = await vscode.commands.executeCommand<vscode.Hover[]>(
          "vscode.executeHoverProvider",
          document.uri,
          at(document, command),
        );
        const markdown = hover
          ?.flatMap((item) => item.contents)
          .map((part) => (typeof part === "string" ? part : part.value))
          .join("\n");
        assert.match(markdown ?? "", description, command);
      }
    },
  );
  for (const [command, word, description] of [
    ["command -p mkdir sample", "mkdir", /make directories/i],
    ["noglob command printf sample", "printf", /NAME/],
    ["builtin printf sample", "printf", /Write formatted output/],
    ["exec -a dummy mkdir sample", "mkdir", /make directories/i],
    ["exec printf sample", "printf", /Print a dummy tile/],
  ] as const) {
    await check(
      `prefix-hover-${command.split(" ")[0]}-${word}`,
      `# Print a dummy tile.\nprintf() { :; }\n${command}\n`,
      async (document) => {
        const hover = await vscode.commands.executeCommand<vscode.Hover[]>(
          "vscode.executeHoverProvider",
          document.uri,
          at(document, word),
        );
        const markdown = hover
          ?.flatMap((item) => item.contents)
          .map((part) => (typeof part === "string" ? part : part.value))
          .join("\n");
        assert.match(markdown ?? "", description, command);
      },
    );
  }
  for (const [name, text, expected] of [
    [
      "builtin",
      `builtin autoload -Uz '${firstPath}/paint_tile'\npaint_tile`,
      firstPath,
    ],
    [
      "quoted",
      `'autoload' -Uz '${firstPath}/paint_tile'\npaint_tile`,
      firstPath,
    ],
    [
      "local-path",
      `load_tiles() { local fpath=('${firstPath}'); autoload -Uz paint_tile; paint_tile; }`,
      firstPath,
    ],
    [
      "inherited-autoload",
      `autoload -Uz paint_tile\nload_tiles() { local fpath=('${firstPath}'); paint_tile; }`,
      firstPath,
    ],
    [
      "path-shadow",
      `fpath=('${secondPath}')\nautoload -Uz paint_tile\nload_tiles() { local fpath=('${firstPath}'); paint_tile; }`,
      firstPath,
    ],
    [
      "global-path",
      `load_tiles() { autoload -Uz paint_tile; paint_tile; }\nfpath=('${firstPath}')`,
      firstPath,
    ],
    [
      "dynamic-path",
      `fpath=('${firstPath}')\nautoload -Uz paint_tile\nload_tiles() { local fpath=($input); paint_tile; }`,
      "",
    ],
    [
      "separate-scopes",
      `load_tiles() { autoload -Uz '${firstPath}/paint_tile'; paint_tile; }\nother_tiles() { autoload -Uz '${secondPath}/paint_tile'; }`,
      firstPath,
    ],
    [
      "unexecuted-scope",
      `unused() { autoload -Uz '${firstPath}/paint_tile'; }\npaint_tile\n`,
      "",
    ],
  ]) {
    await check(`autoload-${name}`, text, async (document) => {
      const reference = text.includes("paint_tile;")
        ? "paint_tile;"
        : "paint_tile";
      const result = await definitions(document, reference);
      if (!expected) {
        assert.equal(result.length, 0);
        return;
      }
      assert.equal(result.length, 1);
      assert.equal(result[0].uri.fsPath, path.join(expected, "paint_tile"));
      assert.equal(result[0].range.start.line, 1);
      const hover = await vscode.commands.executeCommand<vscode.Hover[]>(
        "vscode.executeHoverProvider",
        document.uri,
        at(document, reference),
      );
      assert.ok(hover?.length);
    });
  }
  for (const [name, statement] of [
    ["if", "if [[ $choice == yes ]]; then\n draw_tile\nfi"],
    ["while", "while [[ $choice == yes ]]; do\n draw_tile\ndone"],
    ["for", "for label in amber blue; do\n draw_tile\ndone"],
    ["case", "case $choice in\n yes) draw_tile;;\nesac"],
    ["subshell", "(draw_tile)"],
    ["substitution", "result=$(draw_tile)"],
    ["pipeline", "draw_tile | cat"],
    ["process-substitution", "cat <(draw_tile)"],
  ]) {
    await check(
      `supported-${name}`,
      `draw_tile() { :; }\n${statement}\n`,
      async (document) => {
        await definitionLine(document, "draw_tile", 0);
        const edit = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
          "vscode.executeDocumentRenameProvider",
          document.uri,
          at(document, "draw_tile"),
          "paint_tile",
        );
        assert.ok(edit);
        assert.equal(await vscode.workspace.applyEdit(edit), true);
        assert.equal(
          document.getText(),
          `paint_tile() { :; }\n${statement.replace("draw_tile", "paint_tile")}\n`,
        );
      },
    );
  }

  for (const [name, statement] of [
    ["repeat", "repeat 2 { print sample }"],
    ["always", "{ print before } always { print after }"],
    ["short-for", "for item (amber blue) { print $item }"],
    [
      "multiline",
      `repeat 2 {\n cat <<'TEXT'\nfictional() { :; }\nTEXT\n}\n{\n print sample\n} always {\n print done\n}`,
    ],
  ]) {
    await check(
      `syntax-${name}`,
      `${statement}\ndraw_tile() { :; }\ndraw_tile\n`,
      async (document) => {
        await definitionLine(
          document,
          "draw_tile",
          statement.split("\n").length,
        );
        const symbols = await vscode.commands.executeCommand<
          vscode.DocumentSymbol[]
        >("vscode.executeDocumentSymbolProvider", document.uri);
        assert.ok(!symbols?.some((symbol) => symbol.name === "fictional"));
        assert.deepEqual(vscode.languages.getDiagnostics(document.uri), []);
      },
    );
  }
  const extensionless = path.join(directory, "sample-function");
  await fs.writeFile(extensionless, "draw_tile() { :; }\ndraw_tile\n");
  await check("manual-language", "", async () => {
    const document = await vscode.languages.setTextDocumentLanguage(
      await vscode.workspace.openTextDocument(extensionless),
      "zsh",
    );
    await definitionLine(document, "draw_tile", 0);
  });
  console.log(`Editor audit: ${passed} passed, ${failures.length} failed`);
  if (failures.length)
    throw new AggregateError(failures, "Editor behavior failures");
}
