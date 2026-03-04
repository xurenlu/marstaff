---
name: firecrawl-cli-installation
description: |
  Install the official Firecrawl CLI and handle authentication.
  Package: https://www.npmjs.com/package/firecrawl-cli
  Source: https://github.com/firecrawl/cli
  Docs: https://docs.firecrawl.dev/sdks/cli
---

# Firecrawl CLI Installation

## Quick Setup (Recommended)

```bash
npx -y firecrawl-cli@1.8.0 init --all --browser
```

This installs `firecrawl-cli` globally and authenticates.

## Manual Install

```bash
npm install -g firecrawl-cli@1.8.0
```

## Verify

```bash
firecrawl --status
```

## Authentication

Authenticate using the built-in login flow:

```bash
firecrawl login --browser
```

This opens the browser for OAuth authentication. Credentials are stored securely by the CLI.

### Background / One-off tasks (afk_create_oneoff_task)

When Firecrawl runs via marstaff's **挂机任务** (one-off background tasks), there is no TTY for interactive login. You **must** set `FIRECRAWL_API_KEY` in the environment before the task runs:

```bash
export FIRECRAWL_API_KEY="fc-xxx"   # Get key from firecrawl.dev
```

Or add it to the gateway's environment (e.g. systemd, `.env`, or shell profile used by the service).

### If authentication fails

Ask the user how they'd like to authenticate:

1. **Login with browser (Recommended)** - Run `firecrawl login --browser`
2. **Enter API key manually** - Run `firecrawl login --api-key "<key>"` with a key from firecrawl.dev

### Command not found

If `firecrawl` is not found after installation:

1. Ensure npm global bin is in PATH
2. Try: `npx firecrawl-cli@1.8.0 --version`
3. Reinstall: `npm install -g firecrawl-cli@1.8.0`
