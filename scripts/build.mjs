import { execFileSync } from "node:child_process";
import { mkdir, readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { build } from "esbuild";

async function main() {
  const targets = process.argv.includes("--all")
    ? ["darwin-arm64", "darwin-x64", "linux-arm64", "linux-x64"]
    : [`${process.platform}-${process.arch}`];

  for (const target of targets) {
    const [platform, arch] = target.split("-");
    const dir = `bin/${target}`;
    await mkdir(dir, { recursive: true });
    execFileSync(
      process.env.GO ?? "go",
      [
        "build",
        "-trimpath",
        "-ldflags=-s -w",
        "-o",
        `${dir}/zsh-language-server`,
        "./cmd/zsh-language-server",
      ],
      {
        stdio: "inherit",
        env: {
          ...process.env,
          CGO_ENABLED: "0",
          GOOS: platform,
          GOARCH: arch === "x64" ? "amd64" : arch,
        },
      },
    );
  }

  const result = await build({
    entryPoints: ["client/extension.ts"],
    bundle: true,
    platform: "node",
    format: "cjs",
    target: "node20",
    outfile: "dist/extension.cjs",
    external: ["vscode"],
    metafile: true,
  });

  const notices = [];
  const packages = new Set(
    Object.keys(result.metafile.inputs).flatMap((input) => {
      const match = input.match(/^(.*node_modules\/(?:@[^/]+\/)?[^/]+)/);
      return match ? [match[1]] : [];
    }),
  );
  for (const directory of [...packages].sort()) {
    const metadata = JSON.parse(
      await readFile(path.join(directory, "package.json"), "utf8"),
    );
    notices.push(`${metadata.name} ${metadata.version}`);
    for (const name of await readdir(directory)) {
      if (/^(licen[cs]e|notice)(\..*)?$/i.test(name))
        notices.push(await readFile(path.join(directory, name), "utf8"));
    }
  }
  const modules = execFileSync(
    process.env.GO ?? "go",
    ["list", "-m", "-f", "{{.Path}}\t{{.Version}}\t{{.Dir}}", "all"],
    { encoding: "utf8" },
  );
  for (const line of modules.trim().split("\n").slice(1)) {
    const [name, version, directory] = line.split("\t");
    notices.push(
      `${name} ${version}`,
      await readFile(path.join(directory, "LICENSE"), "utf8"),
    );
  }
  const goroot = execFileSync(process.env.GO ?? "go", ["env", "GOROOT"], {
    encoding: "utf8",
  }).trim();
  notices.push(
    "Go runtime",
    await readFile(path.join(goroot, "LICENSE"), "utf8").catch((error) => {
      if (error.code !== "ENOENT") throw error;
      // Homebrew keeps the license beside libexec, outside GOROOT.
      return readFile(path.join(goroot, "..", "LICENSE"), "utf8");
    }),
  );
  await writeFile(
    "dist/THIRD_PARTY_NOTICES.txt",
    notices.join("\n\n----------------------------------------\n\n"),
  );
}

await main();
