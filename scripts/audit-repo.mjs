import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { parseArgs } from "node:util";
import { openServer } from "../test/lsp-client.mjs";

async function main() {
  const { values } = parseArgs({
    allowNegative: true,
    options: {
      repo: { type: "string", short: "r" },
      "assume-zsh": { type: "boolean", default: true },
      include: { type: "string", short: "i", multiple: true, default: [] },
    },
  });
  const root = path.resolve(values.repo);
  const files = execFileSync(
    "git",
    [
      "-C",
      root,
      "ls-files",
      "-z",
      "--cached",
      "--others",
      "--exclude-standard",
    ],
    {
      encoding: "utf8",
    },
  )
    .split("\0")
    .filter(Boolean);
  const session = await openServer([
    { uri: pathToFileURL(root).href, name: path.basename(root) },
  ]);
  const { rpc, diagnostics } = session;
  const counts = {
    zsh: 0,
    symbols: 0,
    failures: 0,
  };
  try {
    for (const filename of files) {
      const full = path.join(root, filename);
      const text = await readFile(full, "utf8");
      let zsh =
        /^#!\s*(?:\S*\/)?(?:zsh|env\s+(?:-S\s+)?zsh)(?:\s|$)/.test(text) ||
        /\.zsh$|(?:^|\/)\.?z(?:shrc|shenv|profile|login|logout)$/.test(
          filename,
        ) ||
        values.include.some((pattern) => path.matchesGlob(filename, pattern));
      if (!zsh && filename.endsWith(".sh") && values["assume-zsh"]) {
        zsh = !text.startsWith("#!");
        for (const line of text.split("\n")) {
          const trimmed = line.trim();
          if (trimmed && !trimmed.startsWith("#")) break;
          if (/^#\s*shellcheck(?:\s|$)/.test(trimmed)) {
            zsh = false;
            break;
          }
        }
      }
      if (!zsh) continue;
      const uri = pathToFileURL(full).href;
      try {
        counts.zsh++;
        // NO_EQUALS avoids command-path expansion in dynamic declarations during Zsh's no-exec pass.
        const result = spawnSync("zsh", ["-dfn", "-o", "NO_EQUALS"], {
          input: text,
          encoding: "utf8",
          timeout: 5000,
        });
        assert.equal(result.status, 0, result.error?.message ?? result.stderr);
        await rpc.sendNotification("textDocument/didOpen", {
          textDocument: { uri, version: 1, languageId: "zsh", text },
        });
        const symbols = await rpc.sendRequest("textDocument/documentSymbol", {
          textDocument: { uri },
        });
        assert.deepEqual(
          diagnostics.get(uri).diagnostics,
          [],
          "Unexpected parser diagnostic",
        );
        const lines = text.split("\n");
        for (const symbol of symbols) {
          const range = symbol.selectionRange;
          assert.equal(range.start.line, range.end.line);
          assert.equal(
            lines[range.start.line].slice(
              range.start.character,
              range.end.character,
            ),
            symbol.name,
            "Incorrect symbol range",
          );
          counts.symbols++;
        }
        await rpc.sendNotification("textDocument/didClose", {
          textDocument: { uri },
        });
      } catch (error) {
        counts.failures++;
        console.error(`${filename}: ${error.message}`);
      }
    }
  } finally {
    await session.close();
  }
  console.log(JSON.stringify(counts));
  if (counts.failures) process.exitCode = 1;
}

await main();
