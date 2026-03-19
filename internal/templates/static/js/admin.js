/**
 * Admin Pages JavaScript
 * Functions for admin interface interactions
 */

import { copyToClipboard } from './utils.js';

/**
 * Copy client secret to clipboard
 */
function copySecret() {
  var secretElement = document.getElementById('clientSecret');
  if (!secretElement) {
    console.error('Secret element not found');
    return;
  }
  copyToClipboard(secretElement.textContent);
}

/**
 * Toggle client description in table
 */
function toggleDescription(button) {
  var cell = button.closest('.client-name-cell');
  if (!cell) return;

  var description = cell.querySelector('.client-description');
  if (!description) return;

  if (description.style.display === 'none' || !description.style.display) {
    description.style.display = 'block';
    button.classList.add('expanded');

    description.style.maxHeight = '0';
    description.style.overflow = 'hidden';
    description.style.transition = 'max-height 0.3s ease-out';

    description.offsetHeight;

    description.style.maxHeight = description.scrollHeight + 'px';
  } else {
    description.style.maxHeight = '0';
    button.classList.remove('expanded');

    setTimeout(function() {
      description.style.display = 'none';
    }, 300);
  }
}

export { copySecret, toggleDescription };

/**
 * Initialize admin page interactions
 */
document.addEventListener('DOMContentLoaded', function() {
  var secretElements = document.querySelectorAll('.secret-value, .secret-value-enhanced');
  secretElements.forEach(function(element) {
    element.addEventListener('click', function() {
      var range = document.createRange();
      range.selectNodeContents(element);
      var selection = window.getSelection();
      selection.removeAllRanges();
      selection.addRange(range);
    });
  });
});
