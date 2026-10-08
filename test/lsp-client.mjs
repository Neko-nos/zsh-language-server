import { spawn } from "node:child_process";
import { once } from "node:events";
import { fileURLToPath } from "node:url";
import {
  createMessageConnection,
  StreamMessageReader,
  StreamMessageWriter,
} from "vscode-jsonrpc/node";

export async function openServer(workspaceFolders = []) {
  const server = spawn(
    fileURLToPath(
      new URL(
        `../bin/${process.platform}-${process.arch}/zsh-language-server`,
        import.meta.url,
      ),
    ),
  );
  server.stderr.pipe(process.stderr);
  const exited = once(server, "exit");
  const rpc = createMessageConnection(
    new StreamMessageReader(server.stdout),
    new StreamMessageWriter(server.stdin),
  );
  const diagnostics = new Map();
  rpc.onNotification("textDocument/publishDiagnostics", (params) =>
    diagnostics.set(params.uri, params),
  );
  rpc.listen();
  await rpc.sendRequest("initialize", { workspaceFolders });
  await rpc.sendNotification("initialized", {});
  return {
    rpc,
    diagnostics,
    async close() {
      await rpc.sendRequest("shutdown");
      await rpc.sendNotification("exit");
      const [code] = await exited;
      rpc.dispose();
      if (code !== 0) throw new Error(`Server exited with ${code}`);
    },
  };
}
