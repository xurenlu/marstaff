/**
 * LocalStorage wrapper for Marstaff
 * Provides a clean interface for localStorage operations with namespacing
 */

const STORAGE_PREFIX = 'marstaff_';

// Storage wrapper class
class Storage {
    /**
     * Get a value from localStorage
     * @param {string} key - The key to retrieve (without prefix)
     * @param {*} defaultValue - Default value if key doesn't exist
     * @returns {*} The stored value or defaultValue
     */
    get(key, defaultValue = null) {
        try {
            const fullKey = STORAGE_PREFIX + key;
            const item = localStorage.getItem(fullKey);
            if (item === null) {
                return defaultValue;
            }
            // Try to parse as JSON, fall back to string
            try {
                return JSON.parse(item);
            } catch {
                return item;
            }
        } catch (error) {
            console.error('Storage.get failed:', error);
            return defaultValue;
        }
    }

    /**
     * Set a value in localStorage
     * @param {string} key - The key to set (without prefix)
     * @param {*} value - The value to store (will be JSON stringified if not a string)
     */
    set(key, value) {
        try {
            const fullKey = STORAGE_PREFIX + key;
            const item = typeof value === 'string' ? value : JSON.stringify(value);
            localStorage.setItem(fullKey, item);
        } catch (error) {
            console.error('Storage.set failed:', error);
        }
    }

    /**
     * Remove a value from localStorage
     * @param {string} key - The key to remove (without prefix)
     */
    remove(key) {
        try {
            const fullKey = STORAGE_PREFIX + key;
            localStorage.removeItem(fullKey);
        } catch (error) {
            console.error('Storage.remove failed:', error);
        }
    }

    /**
     * Clear all Marstaff values from localStorage
     */
    clear() {
        try {
            const keys = this.keys();
            keys.forEach(key => localStorage.removeItem(STORAGE_PREFIX + key));
        } catch (error) {
            console.error('Storage.clear failed:', error);
        }
    }

    /**
     * Get all Marstaff keys from localStorage
     * @returns {string[]} Array of keys (without prefix)
     */
    keys() {
        try {
            const result = [];
            for (let i = 0; i < localStorage.length; i++) {
                const key = localStorage.key(i);
                if (key && key.startsWith(STORAGE_PREFIX)) {
                    result.push(key.substring(STORAGE_PREFIX.length));
                }
            }
            return result;
        } catch (error) {
            console.error('Storage.keys failed:', error);
            return [];
        }
    }

    /**
     * Check if a key exists in localStorage
     * @param {string} key - The key to check (without prefix)
     * @returns {boolean} True if the key exists
     */
    has(key) {
        try {
            const fullKey = STORAGE_PREFIX + key;
            return localStorage.getItem(fullKey) !== null;
        } catch (error) {
            console.error('Storage.has failed:', error);
            return false;
        }
    }
}

// User Preferences Storage Keys
const StorageKeys = {
    // Mode preferences
    DEFAULT_MODE: 'default_mode',        // 'chat' | 'programming' | 'landing'
    SHOW_LANDING: 'show_landing',        // boolean

    // Session state
    CURRENT_SESSION_ID: 'current_session_id',
    LAST_PROJECT_ID: 'last_project_id',
    LAST_MODE: 'last_mode',              // 'chat' | 'programming'

    // UI preferences
    THEME: 'theme',                      // 'light' | 'dark' | 'auto'
    SIDEBAR_COLLAPSED: 'sidebar_collapsed', // boolean

    // Editor preferences (programming mode)
    EDITOR_FONT_SIZE: 'editor_font_size',
    EDITOR_TAB_SIZE: 'editor_tab_size',
    EDITOR_WORD_WRAP: 'editor_word_wrap',

    // Chat preferences
    AUTO_TITLE: 'auto_title',            // boolean
    STREAM_RESPONSES: 'stream_responses', // boolean
};

// Export singleton instance and keys
const storage = new Storage();

// Export for use in modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { storage, StorageKeys };
}
