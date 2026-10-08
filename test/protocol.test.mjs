import assert from "node:assert/strict";
import { test } from "node:test";
import { openServer } from "./lsp-client.mjs";

test("ordered incremental edits, diagnostics, and versioned rename over stdio", {
  timeout: 10000,
}, async () => {
  const session = await openServer();
  const { rpc, diagnostics } = session;
  const uri = "file:///dummy/session.zsh";
  const document = { uri };
  const symbols = () =>
    rpc.sendRequest("textDocument/documentSymbol", { textDocument: document });
  try {
    await rpc.sendNotification("textDocument/didOpen", {
      textDocument: {
        uri,
        version: 1,
        languageId: "zsh",
        text: "print -r -- '😀'; name=value\nprint -r -- $name\n",
      },
    });
    assert.equal((await symbols())[0].name, "name");
    assert.deepEqual(diagnostics.get(uri).diagnostics, []);
    await rpc.sendNotification("textDocument/didChange", {
      textDocument: { uri, version: 2 },
      contentChanges: [
        {
          range: {
            start: { line: 0, character: 18 },
            end: { line: 0, character: 22 },
          },
          text: "title",
        },
        {
          range: {
            start: { line: 1, character: 13 },
            end: { line: 1, character: 17 },
          },
          text: "title",
        },
      ],
    });
    const params = {
      textDocument: document,
      position: { line: 1, character: 14 },
    };
    const definitions = await rpc.sendRequest(
      "textDocument/definition",
      params,
    );
    assert.deepEqual(definitions, [
      {
        uri,
        range: {
          start: { line: 0, character: 18 },
          end: { line: 0, character: 23 },
        },
      },
    ]);
    const rename = await rpc.sendRequest("textDocument/rename", {
      ...params,
      newName: "caption",
    });
    assert.deepEqual(rename.documentChanges[0].textDocument, {
      uri,
      version: 2,
    });
    assert.equal(rename.documentChanges[0].edits.length, 2);
    assert.ok(
      rename.documentChanges[0].edits.every(
        (edit) => edit.newText === "caption",
      ),
    );

    await rpc.sendNotification("textDocument/didChange", {
      textDocument: { uri, version: 3 },
      contentChanges: [{ text: 'print "' }],
    });
    await symbols();
    assert.equal(diagnostics.get(uri).version, 3);
    assert.equal(diagnostics.get(uri).diagnostics.length, 1);
    await rpc.sendNotification("textDocument/didClose", {
      textDocument: document,
    });
    await assert.rejects(rpc.sendRequest("example/unknown"), { code: -32601 });
    assert.deepEqual(diagnostics.get(uri).diagnostics, []);
  } finally {
    await session.close();
  }
});
