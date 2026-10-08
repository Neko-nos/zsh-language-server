import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, readdir, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import {
  downloadAndUnzipVSCode,
  resolveCliPathFromVSCodeExecutablePath,
  runTests,
} from "@vscode/test-electron";
import { build } from "esbuild";

async function main() {
  const manifest = JSON.parse(await readFile("package.json", "utf8"));
  await build({
    entryPoints: ["test/vscode.test.ts"],
    bundle: true,
    platform: "node",
    format: "cjs",
    target: "node20",
    outfile: "dist/test.cjs",
    external: ["vscode"],
  });
  const vscodeExecutablePath =
    process.env.VSCODE_EXECUTABLE_PATH ?? (await downloadAndUnzipVSCode());
  const directory = await mkdtemp(
    path.join(os.tmpdir(), "zsh-extension-test-"),
  );
  const harness = await mkdtemp(path.join(os.tmpdir(), "zsh-test-harness-"));
  for (const folder of ["zsh", "shell", "user/User"])
    await mkdir(path.join(directory, folder), { recursive: true });
  const workspace = path.join(directory, "test.code-workspace");
  await writeFile(
    workspace,
    JSON.stringify({ folders: [{ path: "zsh" }, { path: "shell" }] }),
  );
  await writeFile(
    path.join(directory, "user/User/settings.json"),
    JSON.stringify({
      "bashIde.shellcheckPath": process.env.SHELLCHECK,
      "editor.wordBasedSuggestions": "off",
    }),
  );
  await writeFile(
    path.join(harness, "package.json"),
    JSON.stringify({
      name: "zsh-test-harness",
      version: "0.0.1",
      publisher: "test-publisher",
      engines: { vscode: "^1.91.0" },
    }),
  );
  const cli = resolveCliPathFromVSCodeExecutablePath(vscodeExecutablePath);
  const launchArgs = [
    "--user-data-dir",
    path.join(directory, "user"),
    "--extensions-dir",
    path.join(directory, "extensions"),
  ];
  for (const extension of [
    path.resolve("zsh-language-server-0.1.0.vsix"),
    "mads-hartmann.bash-ide-vscode@1.43.2",
  ])
    execFileSync(cli, [...launchArgs, "--install-extension", extension], {
      stdio: "inherit",
    });
  await runTests({
    vscodeExecutablePath,
    extensionDevelopmentPath: harness,
    extensionTestsPath: path.resolve("dist/test.cjs"),
    extensionTestsEnv: {
      ZSH_EXTENSION_ID: `${manifest.publisher}.${manifest.name}`,
    },
    launchArgs: [...launchArgs, workspace],
  });
  const logs = path.join(directory, "user", "logs");
  for (const filename of await readdir(logs, { recursive: true })) {
    if (
      filename.endsWith("-Zsh Language Server.log") ||
      filename.endsWith("Bash IDE.log")
    ) {
      assert.doesNotMatch(
        (await readFile(path.join(logs, filename), "utf8")).replace(
          // The editor test deliberately attempts a rename that captures a local parameter.
          /Request textDocument\/rename failed\.\n {2}Message: the new name is already used in an affected scope\n {2}Code: -32803 /g,
          "",
        ),
        /Request \S+ failed/,
        `Unexpected language-server request failure in ${filename}`,
      );
    }
  }
}

await main();
