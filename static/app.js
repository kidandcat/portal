// Tab switching
function switchTab(tabName) {
    var url = new URL(window.location);
    url.searchParams.set('tab', tabName);
    history.pushState({}, '', url);

    document.querySelectorAll('.tab[data-tab]').forEach(function(t) {
        t.classList.toggle('active', t.dataset.tab === tabName);
    });

    document.querySelectorAll('.tab-panel').forEach(function(p) {
        p.classList.toggle('active', p.id === 'tab-' + tabName);
    });
}

// Handle browser back/forward
window.addEventListener('popstate', function() {
    var params = new URLSearchParams(window.location.search);
    var tab = params.get('tab') || 'issues';
    switchTab(tab);
});

// Init tabs on page load
document.addEventListener('DOMContentLoaded', function() {
    document.querySelectorAll('.tab[data-tab]').forEach(function(t) {
        t.addEventListener('click', function(e) {
            e.preventDefault();
            switchTab(this.dataset.tab);
        });
    });
});

// Image carousel
var carouselIndex = 0;

function carouselNav(dir) {
    var slides = document.querySelectorAll('.carousel-slide');
    if (slides.length === 0) return;
    carouselIndex = (carouselIndex + dir + slides.length) % slides.length;
    carouselGo(carouselIndex);
}

function carouselGo(idx) {
    var slides = document.querySelectorAll('.carousel-slide');
    var dots = document.querySelectorAll('.carousel-dot');
    if (slides.length === 0) return;
    carouselIndex = idx;
    slides.forEach(function(s, i) {
        s.classList.toggle('active', i === idx);
    });
    dots.forEach(function(d, i) {
        d.classList.toggle('active', i === idx);
    });
}
