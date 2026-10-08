import * as assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as vscode from "vscode";
import { runEditorTests } from "./editor.test";
import { runWorkflowTests } from "./workflow.test";

async function diagnosticsChanged(uri: vscode.Uri, edit: () => Promise<void>) {
  let subscription: vscode.Disposable | undefined;
  let timeout: ReturnType<typeof setTimeout> | undefined;
  const event = new Promise<void>((resolve, reject) => {
    timeout = setTimeout(
      () => reject(new Error("Diagnostics did not update")),
      10000,
    );
    subscription = vscode.languages.onDidChangeDiagnostics((event) => {
      if (event.uris.some((changed) => changed.toString() === uri.toString()))
        resolve();
    });
  });
  try {
    await edit();
    await event;
  } finally {
    clearTimeout(timeout);
    subscription?.dispose();
  }
}

async function waitFor(condition: () => boolean, message: string) {
  for (let attempt = 0; attempt < 200; attempt++) {
    if (condition()) return;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  assert.fail(message);
}

async function openLanguage(uri: vscode.Uri, language: string) {
  await vscode.workspace.openTextDocument(uri);
  await waitFor(
    () =>
      vscode.workspace.textDocuments.some(
        (document) =>
          document.uri.toString() === uri.toString() &&
          document.languageId === language &&
          !document.isClosed,
      ),
    `Expected ${language}: ${uri.fsPath}`,
  );
  return vscode.workspace.openTextDocument(uri);
}

export async function run() {
  const folders = vscode.workspace.workspaceFolders;
  assert.equal(folders?.length, 2);
  assert.ok(folders);
  const directory = folders[0].uri.fsPath;
  await runWorkflowTests(directory);
  const shellDocuments: vscode.Uri[] = [];
  const main = vscode.Uri.file(path.join(directory, "example.sh"));
  const library = vscode.Uri.file(path.join(directory, "library.zsh"));
  await fs.writeFile(
    library.fsPath,
    "library_helper() { print -r -- example; }\n",
  );
  await fs.writeFile(
    main.fsPath,
    [
      "#!/usr/bin/env zsh",
      "source ./library.zsh",
      "typeset -a colors=(blue green)",
      "# Show the example colors.",
      "show_colors() {",
      "local label=example",
      `print -r -- "\${(U)colors}" "$label"`,
      "}",
      "show_colors",
      "library_helper",
      "",
    ].join("\n"),
  );
  const document = await openLanguage(main, "zsh");
  await vscode.window.showTextDocument(document);

  const symbols = await vscode.commands.executeCommand<vscode.DocumentSymbol[]>(
    "vscode.executeDocumentSymbolProvider",
    main,
  );
  assert.ok(symbols?.some((symbol) => symbol.name === "show_colors"));
  assert.ok(symbols?.some((symbol) => symbol.name === "colors"));
  assert.equal(vscode.languages.getDiagnostics(main).length, 0);

  const definitions = await vscode.commands.executeCommand<vscode.Location[]>(
    "vscode.executeDefinitionProvider",
    main,
    new vscode.Position(8, 3),
  );
  assert.equal(definitions?.[0].range.start.line, 4);
  const sourceDefinitions = await vscode.commands.executeCommand<
    vscode.Location[]
  >("vscode.executeDefinitionProvider", main, new vscode.Position(9, 3));
  assert.equal(sourceDefinitions?.[0].uri.toString(), library.toString());
  const sourceFile = await vscode.commands.executeCommand<vscode.Location[]>(
    "vscode.executeDefinitionProvider",
    main,
    new vscode.Position(1, 12),
  );
  assert.equal(sourceFile?.[0].uri.toString(), library.toString());

  const autoloadDirectory = path.join(path.dirname(directory), "autoload");
  await fs.mkdir(path.join(autoloadDirectory, "helpers"), { recursive: true });
  const autoloadText = [
    `typeset helpers="\${\${(%):-%N}:A:h}/helpers"`,
    `fpath=("$helpers" "\${fpath[@]}")`,
    "autoload -Uz render_sample print_sample log_sample",
    "render_sample",
    "print_sample",
    "",
  ].join("\n");
  const startup = path.join(autoloadDirectory, "startup.zsh");
  await fs.writeFile(startup, autoloadText);
  const startupLink = vscode.Uri.file(
    path.join(path.dirname(directory), ".zshrc"),
  );
  await fs.symlink(startup, startupLink.fsPath);
  for (const [name, body] of [
    [
      "render_sample",
      "# Render a sample.\nrender_sample() {\n  autoload -Uz check_sample\n  check_sample\n  log_sample\n}\n",
    ],
    ["print_sample", "# Print a sample.\nprint sample\n"],
    ["check_sample", "# Check a sample.\nprint checked\n"],
    ["log_sample", "# Log a sample.\nlog_sample() { print logged; }\n"],
  ]) {
    const filename = path.join(autoloadDirectory, "helpers", name);
    await fs.writeFile(filename, body);
  }
  const startupDocument = await openLanguage(startupLink, "zsh");
  for (const name of ["render_sample", "print_sample"]) {
    for (const offset of new Set([
      autoloadText.indexOf(name),
      autoloadText.lastIndexOf(name),
    ])) {
      const definitions = await vscode.commands.executeCommand<
        vscode.Location[]
      >(
        "vscode.executeDefinitionProvider",
        startupLink,
        startupDocument.positionAt(offset),
      );
      assert.equal(definitions?.length, 1);
      assert.equal(
        definitions[0].uri.fsPath,
        await fs.realpath(path.join(autoloadDirectory, "helpers", name)),
      );
      assert.equal(definitions[0].range.start.line, 1);
    }
  }

  const renderUri = vscode.Uri.file(
    path.join(autoloadDirectory, "helpers", "render_sample"),
  );
  const renderDocument = await openLanguage(renderUri, "zsh");
  for (const name of ["check_sample", "log_sample"]) {
    const text = renderDocument.getText();
    for (const offset of new Set([
      text.indexOf(name),
      text.lastIndexOf(name),
    ])) {
      const position = renderDocument.positionAt(offset);
      const definitions = await vscode.commands.executeCommand<
        vscode.Location[]
      >("vscode.executeDefinitionProvider", renderUri, position);
      assert.equal(definitions?.length, 1, `Sibling definition: ${name}`);
      assert.equal(
        definitions[0].uri.fsPath,
        await fs.realpath(path.join(autoloadDirectory, "helpers", name)),
      );
      assert.equal(definitions[0].range.start.line, 1);
      const hovers = await vscode.commands.executeCommand<vscode.Hover[]>(
        "vscode.executeHoverProvider",
        renderUri,
        position,
      );
      assert.ok(hovers?.length, `Sibling hover: ${name}`);
    }
  }
  console.log("Sibling autoload navigation and hover passed.");

  const references = await vscode.commands.executeCommand<vscode.Location[]>(
    "vscode.executeReferenceProvider",
    main,
    new vscode.Position(8, 3),
  );
  assert.equal(references?.length, 2);
  const hover = await vscode.commands.executeCommand<vscode.Hover[]>(
    "vscode.executeHoverProvider",
    main,
    new vscode.Position(8, 3),
  );
  assert.ok(hover?.length);

  const handlerUri = vscode.Uri.file(path.join(directory, "handlers.zsh"));
  const handlerText = [
    "########################",
    "# Draw a dummy label.",
    "# Arguments:",
    "#   1: Label, or `--` for the default.",
    "# Outputs:",
    "#   Writes the `label` in **bold**.",
    "########################",
    "paint_sample() { :; }",
    "paint_sample",
    "compdef paint_sample demo",
    "unset -f paint_sample",
    "print paint_sample",
    "compdef -d paint_sample",
    "",
  ].join("\n");
  await fs.writeFile(handlerUri.fsPath, handlerText);
  const handlerDocument = await openLanguage(handlerUri, "zsh");
  for (const line of [8, 9, 10]) {
    const position = new vscode.Position(
      line,
      handlerDocument.lineAt(line).text.indexOf("paint_sample") + 2,
    );
    const definitions = await vscode.commands.executeCommand<vscode.Location[]>(
      "vscode.executeDefinitionProvider",
      handlerUri,
      position,
    );
    assert.equal(definitions?.length, 1);
    assert.equal(definitions[0].range.start.line, 7);
    const hovers = await vscode.commands.executeCommand<vscode.Hover[]>(
      "vscode.executeHoverProvider",
      handlerUri,
      position,
    );
    const markdown = hovers
      ?.flatMap((hover) => hover.contents)
      .map((content) => (typeof content === "string" ? content : content.value))
      .join("\n");
    assert.ok(markdown?.includes("Draw a dummy label."));
    assert.ok(markdown.includes("### Arguments\n\n- `1`: Label"));
    assert.ok(
      markdown.includes("### Outputs\n\nWrites the `label` in **bold**."),
    );
  }
  for (const line of [11, 12]) {
    const definitions = await vscode.commands.executeCommand<vscode.Location[]>(
      "vscode.executeDefinitionProvider",
      handlerUri,
      new vscode.Position(
        line,
        handlerDocument.lineAt(line).text.indexOf("paint_sample") + 2,
      ),
    );
    assert.equal(definitions?.length, 0);
  }
  const handlerRename =
    await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
      "vscode.executeDocumentRenameProvider",
      handlerUri,
      new vscode.Position(10, 12),
      "draw_sample",
    );
  assert.ok(handlerRename);
  assert.equal(handlerRename.get(handlerUri).length, 4);
  await diagnosticsChanged(handlerUri, async () => {
    assert.equal(await vscode.workspace.applyEdit(handlerRename), true);
  });
  assert.equal(
    handlerDocument.getText(),
    handlerText
      .replace("paint_sample()", "draw_sample()")
      .replace("\npaint_sample\n", "\ndraw_sample\n")
      .replace("compdef paint_sample demo", "compdef draw_sample demo")
      .replace("unset -f paint_sample", "unset -f draw_sample"),
  );

  const completion =
    await vscode.commands.executeCommand<vscode.CompletionList>(
      "vscode.executeCompletionItemProvider",
      main,
      new vscode.Position(8, 4),
    );
  const suggestion = completion?.items.find(
    (item) => item.label === "show_colors",
  );
  assert.ok(suggestion);
  const replacement = suggestion.range;
  assert.ok(replacement instanceof vscode.Range);
  assert.equal(typeof suggestion.insertText, "string");
  const original = document.getText();
  const completionEdit = new vscode.WorkspaceEdit();
  completionEdit.replace(main, replacement, suggestion.insertText as string);
  assert.equal(await vscode.workspace.applyEdit(completionEdit), true);
  assert.equal(document.getText(), original);
  const folds = await vscode.commands.executeCommand<vscode.FoldingRange[]>(
    "vscode.executeFoldingRangeProvider",
    main,
  );
  assert.ok(folds?.some((fold) => fold.start === 4));

  const rename = await vscode.commands.executeCommand<vscode.WorkspaceEdit>(
    "vscode.executeDocumentRenameProvider",
    main,
    new vscode.Position(8, 3),
    "display_colors",
  );
  assert.equal(rename?.get(main).length, 2);
  assert.ok(rename);
  await diagnosticsChanged(main, async () => {
    assert.equal(await vscode.workspace.applyEdit(rename), true);
  });
  assert.match(document.getText(), /display_colors\(\)/);

  await diagnosticsChanged(main, async () => {
    const edit = new vscode.WorkspaceEdit();
    edit.insert(
      main,
      document.positionAt(document.getText().length),
      'print "',
    );
    assert.equal(await vscode.workspace.applyEdit(edit), true);
  });
  assert.equal(vscode.languages.getDiagnostics(main).length, 1);
  await diagnosticsChanged(main, async () => {
    const edit = new vscode.WorkspaceEdit();
    edit.insert(
      main,
      document.positionAt(document.getText().length),
      'example"\n',
    );
    assert.equal(await vscode.workspace.applyEdit(edit), true);
  });
  assert.equal(vscode.languages.getDiagnostics(main).length, 0);

  for (const [name, contents] of [
    [".zshrc", "print -r -- example\n"],
    ["script.fixture", "#!/usr/bin/env zsh\nprint -r -- example\n"],
    ["script.sh", "#!/usr/bin/env zsh\nprint -r -- example\n"],
    ["split.sh", "#!/usr/bin/env -S zsh -f\nprint example\n"],
    ["shared.sh", "sample=value\nprint -r -- $sample\n"],
    [
      "zsh-shellcheck.sh",
      "#!/usr/bin/env zsh\n# shellcheck shell=bash\nprint example\n",
    ],
    [
      "later-comment.sh",
      "print example\n# shellcheck disable=SC2086\nprint example\n",
    ],
    ["quoted-comment.sh", "cat <<'TEXT'\n# shellcheck shell=bash\nTEXT\n"],
  ]) {
    const filename = path.join(directory, name);
    await fs.writeFile(filename, contents);
    await openLanguage(vscode.Uri.file(filename), "zsh");
  }
  for (const [name, contents] of [
    ["bash.sh", "#!/usr/bin/env bash\nprintf '%s\\n' example\n"],
    ["posix.sh", "#!/bin/sh\nprintf '%s\\n' example\n"],
    ["shellcheck.sh", "# shellcheck shell=bash\nprintf '%s\\n' example\n"],
    [
      "shellcheck-disable.sh",
      "# Dummy helper.\n\n  # shellcheck disable=SC2086\nprintf '%s\\n' example\n",
    ],
    ["bash-argument.sh", "#!/bin/bash zsh\nprintf example\n"],
  ]) {
    const filename = path.join(directory, name);
    await fs.writeFile(filename, contents);
    const shell = await openLanguage(vscode.Uri.file(filename), "shellscript");
    shellDocuments.push(shell.uri);
    assert.deepEqual(vscode.languages.getDiagnostics(shell.uri), []);
  }
  const bashExtension = vscode.extensions.getExtension(
    "mads-hartmann.bash-ide-vscode",
  );
  assert.ok(bashExtension);
  await bashExtension.activate();
  const body =
    'sample_name="alpha beta"\nshow_sample() { echo $sample_name; }\nshow_sample\n';
  async function shellFeatures(uri: vscode.Uri, language: string) {
    const document = await openLanguage(uri, language);
    const definitions = await vscode.commands.executeCommand<vscode.Location[]>(
      "vscode.executeDefinitionProvider",
      uri,
      new vscode.Position(document.lineCount - 2, 3),
    );
    assert.equal(definitions?.length, 1);
    const completion =
      await vscode.commands.executeCommand<vscode.CompletionList>(
        "vscode.executeCompletionItemProvider",
        uri,
        new vscode.Position(document.lineCount - 2, 5),
      );
    assert.equal(
      completion?.items.filter((item) => item.label === "show_sample").length,
      1,
    );
    await waitFor(() => {
      const diagnostics = vscode.languages.getDiagnostics(uri);
      return language === "zsh"
        ? diagnostics.length === 0
        : diagnostics.some((diagnostic) => {
            const code =
              typeof diagnostic.code === "object"
                ? diagnostic.code.value
                : diagnostic.code;
            return diagnostic.source === "shellcheck" && code === "SC2086";
          });
    }, `Expected diagnostics from ${language}: ${uri.fsPath}`);
  }
  const first = vscode.Uri.file(path.join(directory, "inferred.sh"));
  const second = vscode.Uri.joinPath(folders[1].uri, "inferred.sh");
  for (const uri of [first, second]) {
    await fs.writeFile(uri.fsPath, body);
    await shellFeatures(uri, "zsh");
  }
  await vscode.workspace
    .getConfiguration("zsh")
    .update("assumeZsh", false, vscode.ConfigurationTarget.Workspace);
  for (const uri of [first, second]) await shellFeatures(uri, "shellscript");
  await vscode.workspace
    .getConfiguration("zsh", first)
    .update("assumeZsh", true, vscode.ConfigurationTarget.WorkspaceFolder);
  await shellFeatures(first, "zsh");
  await shellFeatures(second, "shellscript");

  const changing = vscode.Uri.file(path.join(directory, "changing.sh"));
  await fs.writeFile(changing.fsPath, body);
  await shellFeatures(changing, "zsh");
  for (const [header, language] of [
    ["# Dummy helper.\n\n# shellcheck disable=SC2034\n", "shellscript"],
    ["#!/usr/bin/env zsh\n# shellcheck disable=SC2034\n", "zsh"],
    ["#!/usr/bin/env bash\n", "shellscript"],
    ["#!/bin/sh\n", "shellscript"],
    ["", "zsh"],
  ]) {
    const document = await vscode.workspace.openTextDocument(changing);
    const edit = new vscode.WorkspaceEdit();
    edit.replace(
      changing,
      new vscode.Range(
        document.positionAt(0),
        document.positionAt(document.getText().length),
      ),
      header + body,
    );
    assert.equal(await vscode.workspace.applyEdit(edit), true);
    await shellFeatures(changing, language);
  }
  await vscode.workspace.getConfiguration("files").update(
    "associations",
    {
      "*.override.sh": "shellscript",
      "*.sourced": "zsh",
      ".zprofile_template": "zsh",
    },
    vscode.ConfigurationTarget.Global,
  );
  for (const name of ["helper.sourced", ".zprofile_template"]) {
    const uri = vscode.Uri.file(path.join(directory, name));
    await fs.writeFile(uri.fsPath, "sample=value\nprint -r -- $sample\n");
    const sourced = await openLanguage(uri, "zsh");
    const symbols = await vscode.commands.executeCommand<
      vscode.DocumentSymbol[]
    >("vscode.executeDocumentSymbolProvider", sourced.uri);
    assert.equal(symbols?.[0].name, "sample");
  }
  const override = path.join(directory, "script.override.sh");
  await fs.writeFile(
    override,
    "#!/usr/bin/env zsh\n# shellcheck shell=bash\nprintf '%s\\n' example\n",
  );
  assert.equal(
    (await vscode.workspace.openTextDocument(override)).languageId,
    "shellscript",
  );
  shellDocuments.push(vscode.Uri.file(override));
  const untitled = await vscode.workspace.openTextDocument({
    language: "zsh",
    content: "sample=value\nprint -r -- $sample\n",
  });
  const untitledSymbols = await vscode.commands.executeCommand<
    vscode.DocumentSymbol[]
  >("vscode.executeDocumentSymbolProvider", untitled.uri);
  assert.ok(untitledSymbols?.some((symbol) => symbol.name === "sample"));
  // Exercise concurrent requests for dirty and untitled buffers across restarts.
  for (let attempt = 0; attempt < 10; attempt++) {
    await vscode.commands.executeCommand("zsh.restartServer");
    const [restarted, restartedUntitled] = await Promise.all([
      vscode.commands.executeCommand<vscode.DocumentSymbol[]>(
        "vscode.executeDocumentSymbolProvider",
        main,
      ),
      vscode.commands.executeCommand<vscode.DocumentSymbol[]>(
        "vscode.executeDocumentSymbolProvider",
        untitled.uri,
      ),
    ]);
    assert.ok(restarted?.some((symbol) => symbol.name === "display_colors"));
    assert.ok(restartedUntitled?.some((symbol) => symbol.name === "sample"));
  }
  for (const uri of shellDocuments) {
    assert.equal(
      (await vscode.workspace.openTextDocument(uri)).languageId,
      "shellscript",
      uri.fsPath,
    );
    assert.deepEqual(vscode.languages.getDiagnostics(uri), []);
  }
  await runEditorTests(directory);
  console.log(
    "VS Code integration passed: Bash IDE coexistence, header detection and edits, workspace/folder defaults, diagnostics, completion, hover, definitions, autoload navigation through symlinks outside the workspace, sourced-file navigation, references, rename, folding, untitled documents and restart.",
  );
}
