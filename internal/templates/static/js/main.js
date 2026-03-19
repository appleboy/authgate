import { toggleMenu, toggleDropdown } from './navbar.js';
import { copySecret, toggleDescription } from './admin.js';
import {
  formatRelativeTime,
  copyToClipboard,
  showNotification,
  confirmModal,
  toggleDetails,
  toggleTheme,
  toggleFilters
} from './utils.js';
import './code-formatter.js';

// Expose functions globally so HTML onclick attributes can access them
window.toggleMenu = toggleMenu;
window.toggleDropdown = toggleDropdown;
window.copySecret = copySecret;
window.toggleDescription = toggleDescription;
window.formatRelativeTime = formatRelativeTime;
window.copyToClipboard = copyToClipboard;
window.showNotification = showNotification;
window.confirmModal = confirmModal;
window.toggleDetails = toggleDetails;
window.toggleTheme = toggleTheme;
window.toggleFilters = toggleFilters;
