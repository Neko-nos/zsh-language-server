# Zsh Language Server

A VS Code extension for Zsh with syntax highlighting, diagnostics, completion, hover, navigation, and rename.

## Install

With Node.js and Go installed, run:

```sh
npm ci
npm run package
```

In VS Code, run **Extensions: Install from VSIX…** and select `zsh-language-server.vsix`.

### Automatic updates after pulling

Enable automatic updates once in this checkout:

```sh
git config --local hook.update-extension.command ./scripts/update-extension.sh
git config --local hook.update-extension.event post-merge
```

## Use

Open a Zsh file and use VS Code's completion, hover, navigation, and rename commands. The status bar should show **Zsh**. References and rename apply to the current file.

For unrecognized files, select **Zsh** from the status bar's language menu. To remember the choice, add the filename or glob pattern to `files.associations` with the value `zsh` in your workspace settings.

Unmarked `.sh` files default to Zsh; shebangs, leading ShellCheck comments, and explicit file associations take precedence. Set `"zsh.assumeZsh": false` in workspace settings to keep unmarked `.sh` files in Shell Script mode.

To restart, run **Zsh: Restart Language Server**. Logs are in the **Zsh Language Server** Output channel.
