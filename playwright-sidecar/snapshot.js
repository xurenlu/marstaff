/**
 * snapshot.js — DOM 遍历提取可交互元素，生成编号引用映射。
 *
 * 核心思路：注入 JS 到页面，遍历可见 DOM 节点，提取所有可交互元素
 * （link, button, input, select, textarea, [role=button] 等），
 * 为每个元素分配一个递增的 ref 编号，返回格式化文本供 LLM 阅读，
 * 同时在内存中维护 ref → selector 映射供后续操作使用。
 */

const INTERACTIVE_SELECTORS = [
  'a[href]',
  'button',
  'input:not([type="hidden"])',
  'textarea',
  'select',
  '[role="button"]',
  '[role="link"]',
  '[role="checkbox"]',
  '[role="radio"]',
  '[role="tab"]',
  '[role="menuitem"]',
  '[role="option"]',
  '[role="switch"]',
  '[role="combobox"]',
  '[role="searchbox"]',
  '[role="textbox"]',
  '[contenteditable="true"]',
  '[tabindex]:not([tabindex="-1"])',
  'details > summary',
];

const EXTRACT_ELEMENTS_JS = `
(() => {
  const SELECTORS = ${JSON.stringify(INTERACTIVE_SELECTORS)};
  const seen = new Set();
  const elements = [];

  function isVisible(el) {
    if (!el.offsetParent && el.tagName !== 'BODY' && el.tagName !== 'HTML') {
      const style = getComputedStyle(el);
      if (style.position === 'fixed' || style.position === 'sticky') {
        if (style.display === 'none' || style.visibility === 'hidden') return false;
      } else {
        return false;
      }
    }
    const rect = el.getBoundingClientRect();
    if (rect.width === 0 && rect.height === 0) return false;
    const style = getComputedStyle(el);
    if (style.opacity === '0') return false;
    return true;
  }

  function getElementType(el) {
    const tag = el.tagName.toLowerCase();
    const role = el.getAttribute('role');
    if (role) return role;
    if (tag === 'a') return 'link';
    if (tag === 'button' || tag === 'summary') return 'button';
    if (tag === 'select') return 'select';
    if (tag === 'textarea') return 'textarea';
    if (tag === 'input') {
      const type = (el.type || 'text').toLowerCase();
      return 'input[' + type + ']';
    }
    if (el.contentEditable === 'true') return 'textbox';
    return tag;
  }

  function getLabel(el) {
    const ariaLabel = el.getAttribute('aria-label');
    if (ariaLabel) return ariaLabel;

    if (el.id) {
      const label = document.querySelector('label[for="' + CSS.escape(el.id) + '"]');
      if (label) return label.textContent.trim();
    }

    const closestLabel = el.closest('label');
    if (closestLabel) {
      const txt = closestLabel.textContent.trim();
      if (txt && txt.length < 80) return txt;
    }

    return '';
  }

  function getDisplayText(el) {
    const tag = el.tagName.toLowerCase();

    if (tag === 'input' || tag === 'textarea') {
      return el.value || el.placeholder || '';
    }
    if (tag === 'select') {
      const selected = el.options[el.selectedIndex];
      return selected ? selected.text : '';
    }
    if (tag === 'img') {
      return el.alt || '';
    }

    const text = el.textContent || '';
    const trimmed = text.replace(/\\s+/g, ' ').trim();
    return trimmed.length > 100 ? trimmed.substring(0, 100) + '...' : trimmed;
  }

  function buildSelector(el, index) {
    const tag = el.tagName.toLowerCase();
    if (el.id && document.querySelectorAll('#' + CSS.escape(el.id)).length === 1) {
      return '#' + CSS.escape(el.id);
    }
    const dataTestId = el.getAttribute('data-testid');
    if (dataTestId) return '[data-testid="' + dataTestId + '"]';
    return '[data-pw-ref="' + index + '"]';
  }

  const allInteractive = document.querySelectorAll(SELECTORS.join(','));

  for (const el of allInteractive) {
    if (seen.has(el)) continue;
    seen.add(el);
    if (!isVisible(el)) continue;

    const rect = el.getBoundingClientRect();
    const index = elements.length + 1;

    el.setAttribute('data-pw-ref', String(index));

    const info = {
      ref: index,
      type: getElementType(el),
      text: getDisplayText(el),
      label: getLabel(el),
      selector: buildSelector(el, index),
      href: el.tagName.toLowerCase() === 'a' ? el.href : undefined,
      placeholder: el.placeholder || undefined,
      value: (el.tagName.toLowerCase() === 'input' || el.tagName.toLowerCase() === 'textarea') ? el.value : undefined,
      checked: el.type === 'checkbox' || el.type === 'radio' ? el.checked : undefined,
      disabled: el.disabled || false,
      bounds: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      },
    };

    elements.push(info);
  }

  return elements;
})()
`;

class SnapshotManager {
  constructor() {
    this.refMap = new Map();
  }

  /**
   * Take a snapshot of the page, returning formatted text and updating ref mappings.
   * @param {import('playwright').Page} page
   * @returns {Promise<string>}
   */
  async snapshot(page) {
    const title = await page.title().catch(() => '');
    const url = page.url();

    const elements = await page.evaluate(EXTRACT_ELEMENTS_JS);

    this.refMap.clear();
    for (const el of elements) {
      this.refMap.set(el.ref, el.selector);
    }

    const lines = [`Page: ${title}`, `URL: ${url}`, ''];

    if (elements.length === 0) {
      lines.push('(No interactive elements found on this page)');
      return lines.join('\n');
    }

    for (const el of elements) {
      let line = `[${el.ref}] ${el.type}`;

      if (el.label) {
        line += ` "${el.label}"`;
      } else if (el.text) {
        line += ` "${el.text}"`;
      }

      if (el.placeholder && !el.text && !el.label) {
        line += ` placeholder="${el.placeholder}"`;
      }
      if (el.value) {
        line += ` value="${el.value}"`;
      }
      if (el.href) {
        let displayHref = el.href;
        if (displayHref.length > 80) {
          displayHref = displayHref.substring(0, 77) + '...';
        }
        line += ` href="${displayHref}"`;
      }
      if (el.checked !== undefined) {
        line += el.checked ? ' [checked]' : ' [unchecked]';
      }
      if (el.disabled) {
        line += ' [disabled]';
      }

      lines.push(line);
    }

    return lines.join('\n');
  }

  /**
   * Get the CSS selector for a given ref number.
   * @param {number} ref
   * @returns {string|null}
   */
  getSelector(ref) {
    return this.refMap.get(ref) || null;
  }

  /**
   * Get a Playwright locator for a given ref.
   * @param {import('playwright').Page} page
   * @param {number} ref
   * @returns {import('playwright').Locator|null}
   */
  getLocator(page, ref) {
    const selector = this.getSelector(ref);
    if (!selector) return null;
    return page.locator(selector);
  }

  clear() {
    this.refMap.clear();
  }
}

module.exports = { SnapshotManager };
