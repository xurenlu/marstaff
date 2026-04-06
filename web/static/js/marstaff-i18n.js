/**
 * Marstaff UI i18n: loads /static/locales/{locale}.json (en | zh).
 * Locale: localStorage 'marstaff_locale', else navigator (zh* -> zh, else en).
 */
(function () {
    'use strict';

    var dict = {};

    function currentLocale() {
        var stored = localStorage.getItem('marstaff_locale');
        if (stored === 'en' || stored === 'zh') {
            return stored;
        }
        if (typeof navigator !== 'undefined' && navigator.language) {
            return navigator.language.toLowerCase().indexOf('zh') === 0 ? 'zh' : 'en';
        }
        return 'en';
    }

    function loadDict() {
        var locale = currentLocale();
        var url = '/static/locales/' + locale + '.json';
        try {
            var xhr = new XMLHttpRequest();
            xhr.open('GET', url, false);
            xhr.send(null);
            if (xhr.status >= 200 && xhr.status < 300 && xhr.responseText) {
                dict = JSON.parse(xhr.responseText);
            }
        } catch (e) {
            dict = {};
        }
    }

    function t(key) {
        if (dict[key] !== undefined && dict[key] !== null) {
            return String(dict[key]);
        }
        return key;
    }

    function apply() {
        document.querySelectorAll('[data-i18n]').forEach(function (el) {
            var k = el.getAttribute('data-i18n');
            if (k) {
                el.textContent = t(k);
            }
        });
        document.querySelectorAll('[data-i18n-placeholder]').forEach(function (el) {
            var k = el.getAttribute('data-i18n-placeholder');
            if (k) {
                el.placeholder = t(k);
            }
        });
        document.querySelectorAll('[data-i18n-title]').forEach(function (el) {
            var k = el.getAttribute('data-i18n-title');
            if (k) {
                el.title = t(k);
            }
        });
        var titleEl = document.querySelector('title[data-i18n]');
        if (titleEl) {
            var tk = titleEl.getAttribute('data-i18n');
            if (tk) {
                titleEl.textContent = t(tk);
            }
        }
    }

    loadDict();

    window.MarstaffI18n = {
        t: t,
        locale: function () {
            return currentLocale();
        },
        setLocale: function (loc) {
            if (loc !== 'en' && loc !== 'zh') {
                return;
            }
            localStorage.setItem('marstaff_locale', loc);
            loadDict();
            apply();
            window.dispatchEvent(new CustomEvent('marstaff-locale-changed', { detail: { locale: loc } }));
        },
        apply: apply,
        reload: function () {
            loadDict();
            apply();
        }
    };

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', apply);
    } else {
        apply();
    }
})();
