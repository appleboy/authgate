/**
 * Navbar Toggle for Mobile Menu
 */
function toggleMenu() {
  const menu = document.getElementById('navbarMenu');
  if (menu) {
    menu.classList.toggle('active');
  }
}

// Close mobile menu when clicking outside
document.addEventListener('DOMContentLoaded', function() {
  const navbar = document.querySelector('.navbar');
  const navbarMenu = document.getElementById('navbarMenu');
  const navbarToggle = document.querySelector('.navbar-toggle');

  if (navbar && navbarMenu && navbarToggle) {
    document.addEventListener('click', function(event) {
      const isClickInside = navbar.contains(event.target);

      if (!isClickInside && navbarMenu.classList.contains('active')) {
        navbarMenu.classList.remove('active');
      }
    });

    // Close menu when clicking on a link
    const navLinks = navbarMenu.querySelectorAll('.navbar-link');
    navLinks.forEach(link => {
      link.addEventListener('click', function() {
        if (window.innerWidth <= 768) {
          navbarMenu.classList.remove('active');
        }
      });
    });
  }
});
