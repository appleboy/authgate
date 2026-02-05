/**
 * Device Code Auto-Formatting
 * Formats user code input to XXXX-XXXX pattern
 */
document.addEventListener('DOMContentLoaded', function() {
  const codeInput = document.getElementById('user_code');

  if (codeInput) {
    codeInput.addEventListener('input', function(e) {
      let value = e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '');

      if (value.length > 4) {
        value = value.slice(0, 4) + '-' + value.slice(4, 8);
      }

      e.target.value = value;
    });

    // Auto-focus on page load
    codeInput.focus();
  }
});
