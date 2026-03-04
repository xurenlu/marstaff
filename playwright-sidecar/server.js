#!/usr/bin/env node
/**
 * Marstaff Playwright Sidecar — JSON-RPC 2.0 over stdio
 *
 * 从 stdin 逐行读取 JSON-RPC 请求，执行后写入 stdout 一行 JSON 响应。
 * stderr 用于日志，不参与协议。
 */

const readline = require('readline');
const { chromium } = require('playwright');
const { SnapshotManager } = require('./snapshot.js');

const rl = readline.createInterface({ input: process.stdin, output: process.stdout, terminal: false });

let browser = null;
let context = null;
let page = null;
const snapshotMgr = new SnapshotManager();

function log(...args) {
  console.error('[playwright-sidecar]', ...args);
}

function sendResponse(id, result, error) {
  const resp = {
    jsonrpc: '2.0',
    id: id ?? null,
  };
  if (error) {
    resp.error = { code: error.code ?? -32603, message: error.message ?? 'Internal error' };
  } else {
    resp.result = result ?? null;
  }
  process.stdout.write(JSON.stringify(resp) + '\n');
}

async function handleRequest(method, params) {
  switch (method) {
    case 'ping':
      return { ok: true };

    case 'browser.launch': {
      if (browser) {
        return { ok: true, message: 'already running' };
      }
      const headless = params?.headless !== false;
      // Prefer system Chrome (no download). Fallback to bundled chromium if Chrome not found.
      let launchOpts = {
        headless,
        args: [
          '--disable-blink-features=AutomationControlled',
          '--window-size=1920,1080',
        ],
      };
      try {
        browser = await chromium.launch({ ...launchOpts, channel: 'chrome' });
      } catch (e) {
        log('System Chrome not found, using bundled Chromium:', e.message);
        browser = await chromium.launch(launchOpts);
      }
      context = await browser.newContext({
        userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
        viewport: { width: 1920, height: 1080 },
      });
      page = await context.newPage();
      return { ok: true };
    }

    case 'browser.close': {
      if (browser) {
        await browser.close();
        browser = null;
        context = null;
        page = null;
        snapshotMgr.clear();
      }
      return { ok: true };
    }

    case 'page.navigate': {
      ensurePage();
      const url = params?.url ?? '';
      const fullUrl = url && !url.startsWith('http') ? 'https://' + url : url;
      await page.goto(fullUrl, { waitUntil: 'domcontentloaded', timeout: 30000 });
      return { url: page.url(), title: await page.title() };
    }

    case 'page.snapshot': {
      ensurePage();
      const text = await snapshotMgr.snapshot(page);
      return { text };
    }

    case 'page.click': {
      ensurePage();
      const ref = params?.ref;
      if (ref == null) throw { code: -32602, message: 'ref is required' };
      const loc = snapshotMgr.getLocator(page, Number(ref));
      if (!loc) throw { code: -32602, message: 'invalid ref or snapshot expired' };
      await loc.click({ timeout: 5000 });
      return { ok: true };
    }

    case 'page.fill': {
      ensurePage();
      const ref = params?.ref;
      const text = params?.text ?? '';
      if (ref == null) throw { code: -32602, message: 'ref is required' };
      const loc = snapshotMgr.getLocator(page, Number(ref));
      if (!loc) throw { code: -32602, message: 'invalid ref or snapshot expired' };
      await loc.fill(text, { timeout: 5000 });
      return { ok: true };
    }

    case 'page.type': {
      ensurePage();
      const ref = params?.ref;
      const text = params?.text ?? '';
      if (ref == null) throw { code: -32602, message: 'ref is required' };
      const loc = snapshotMgr.getLocator(page, Number(ref));
      if (!loc) throw { code: -32602, message: 'invalid ref or snapshot expired' };
      await loc.fill('');
      await loc.type(text, { delay: 50 });
      return { ok: true };
    }

    case 'page.select': {
      ensurePage();
      const ref = params?.ref;
      const value = params?.value ?? '';
      if (ref == null) throw { code: -32602, message: 'ref is required' };
      const loc = snapshotMgr.getLocator(page, Number(ref));
      if (!loc) throw { code: -32602, message: 'invalid ref or snapshot expired' };
      await loc.selectOption(value);
      return { ok: true };
    }

    case 'page.hover': {
      ensurePage();
      const ref = params?.ref;
      if (ref == null) throw { code: -32602, message: 'ref is required' };
      const loc = snapshotMgr.getLocator(page, Number(ref));
      if (!loc) throw { code: -32602, message: 'invalid ref or snapshot expired' };
      await loc.hover();
      return { ok: true };
    }

    case 'page.screenshot': {
      ensurePage();
      const fullPage = params?.fullPage === true;
      const buf = await page.screenshot({ fullPage, type: 'png' });
      return { base64: buf.toString('base64'), width: 1920, height: 1080 };
    }

    case 'page.getText': {
      ensurePage();
      const selector = params?.selector ?? 'body';
      const text = await page.locator(selector).first().innerText({ timeout: 5000 }).catch(() => '');
      return { text };
    }

    case 'page.getHTML': {
      ensurePage();
      const selector = params?.selector ?? 'body';
      const html = await page.locator(selector).first().innerHTML({ timeout: 5000 }).catch(() => '');
      return { html };
    }

    case 'page.getUrl':
      ensurePage();
      return { url: page.url() };

    case 'page.getTitle':
      ensurePage();
      return { title: await page.title() };

    case 'page.evaluate': {
      ensurePage();
      const script = params?.script ?? '';
      if (!script) throw { code: -32602, message: 'script is required' };
      const result = await page.evaluate(script);
      return { result };
    }

    case 'page.waitForSelector': {
      ensurePage();
      const selector = params?.selector;
      const timeout = params?.timeout ?? 10000;
      if (!selector) throw { code: -32602, message: 'selector is required' };
      await page.waitForSelector(selector, { timeout });
      return { ok: true };
    }

    case 'page.wait': {
      ensurePage();
      const seconds = Math.min(10, Math.max(1, params?.seconds ?? 2));
      await new Promise((r) => setTimeout(r, seconds * 1000));
      return { ok: true };
    }

    case 'page.goBack':
      ensurePage();
      await page.goBack();
      return { ok: true };

    case 'page.goForward':
      ensurePage();
      await page.goForward();
      return { ok: true };

    case 'page.scroll': {
      ensurePage();
      const direction = params?.direction ?? 'down';
      const amount = params?.amount ?? 300;
      const delta = direction === 'up' ? -amount : amount;
      await page.evaluate((d) => window.scrollBy(0, d), delta);
      return { ok: true };
    }

    case 'page.pressKey': {
      ensurePage();
      const key = params?.key ?? '';
      if (!key) throw { code: -32602, message: 'key is required' };
      await page.keyboard.press(key);
      return { ok: true };
    }

    case 'page.tabs': {
      ensurePage();
      const pages = context.pages();
      const tabs = await Promise.all(pages.map(async (p, i) => ({
        id: i,
        url: p.url(),
        title: await p.title().catch(() => p.url()),
      })));
      return { tabs };
    }

    case 'page.switchTab': {
      ensurePage();
      const id = params?.id ?? 0;
      const pages = context.pages();
      if (id < 0 || id >= pages.length) throw { code: -32602, message: 'invalid tab id' };
      page = pages[id];
      return { ok: true };
    }

    default:
      throw { code: -32601, message: `Method not found: ${method}` };
  }
}

function ensurePage() {
  if (!page) {
    throw { code: -1, message: 'browser not launched. call browser.launch first' };
  }
}

rl.on('line', async (line) => {
  const trimmed = line.trim();
  if (!trimmed) return;

  let req;
  try {
    req = JSON.parse(trimmed);
  } catch (e) {
    sendResponse(null, null, { code: -32700, message: 'Parse error' });
    return;
  }

  const id = req.id;
  const method = req.method;
  const params = req.params ?? {};

  if (!method) {
    sendResponse(id, null, { code: -32600, message: 'Invalid Request' });
    return;
  }

  try {
    const result = await handleRequest(method, params);
    sendResponse(id, result);
  } catch (err) {
    const errObj = typeof err === 'object' && err !== null ? err : { message: String(err) };
    sendResponse(id, null, { code: errObj.code ?? -32603, message: errObj.message ?? 'Internal error' });
  }
});

rl.on('close', async () => {
  if (browser) {
    try {
      await browser.close();
    } catch (e) {
      log('close error:', e);
    }
  }
  process.exit(0);
});

process.on('SIGTERM', async () => {
  if (browser) await browser.close();
  process.exit(0);
});

process.on('SIGINT', async () => {
  if (browser) await browser.close();
  process.exit(0);
});

log('Playwright sidecar ready, reading JSON-RPC from stdin');
