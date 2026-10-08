import * as assert from "node:assert/strict";
import * as fs from "node:fs/promises";
import * as path from "node:path";
import * as vscode from "vscode";

export async function runWorkflowTests(directory: string) {
  const library = path.join(directory, "helpers");
  await fs.mkdir(library);
  const entry = vscode.Uri.file(path.join(library, "compose_tile"));
  const helper = vscode.Uri.file(path.join(library, "paint_tile"));
  await fs.writeFile(entry.fsPath, "compose_tile() {\n  paint_tile\n}\n");
  await fs.writeFile(helper.fsPath, "# Paint a tile.\npaint_tile() { :; }\n");
  await vscode.workspace
    .getConfiguration("files")
    .update(
      "associations",
      { "**/helpers/*": "zsh" },
      vscode.ConfigurationTarget.Global,
    );
  const document = await vscode.workspace.openTextDocument(entry);
  assert.equal(document.languageId, "zsh");
  await vscode.window.showTextDocument(document);
  const extensionId = process.env.ZSH_EXTENSION_ID;
  assert.ok(extensionId);
  const extension = vscode.extensions.getExtension(extensionId);
  assert.ok(extension);
  await extension.activate();
  async function checkDefinition(uri: vscode.Uri, line: number) {
    const result = await vscode.commands.executeCommand<vscode.Location[]>(
      "vscode.executeDefinitionProvider",
      entry,
      new vscode.Position(1, 4),
    );
    assert.equal(result?.length, 1);
    assert.equal(result[0].uri.fsPath, await fs.realpath(uri.fsPath));
    assert.equal(result[0].range.start.line, line);
  }
  await checkDefinition(helper, 1);
  await vscode.workspace.openTextDocument(helper);
  const edit = new vscode.WorkspaceEdit();
  edit.insert(helper, new vscode.Position(0, 0), "\n\n");
  assert.equal(await vscode.workspace.applyEdit(edit), true);
  await checkDefinition(helper, 3);
  const moved = vscode.Uri.file(path.join(library, "draw_tile"));
  const rename = new vscode.WorkspaceEdit();
  rename.renameFile(helper, moved);
  assert.equal(await vscode.workspace.applyEdit(rename), true);
  const movedDocument = await vscode.workspace.openTextDocument(moved);
  const update = new vscode.WorkspaceEdit();
  update.replace(moved, new vscode.Range(3, 0, 3, 10), "draw_tile");
  update.replace(entry, new vscode.Range(1, 2, 1, 12), "draw_tile");
  assert.equal(await vscode.workspace.applyEdit(update), true);
  await checkDefinition(moved, 3);
  assert.equal(await movedDocument.save(), true);
  await vscode.commands.executeCommand("zsh.restartServer");
  await checkDefinition(moved, 3);
  console.log(
    "Cold-open, unsaved edits, file moves, and restart workflow passed.",
  );
}
