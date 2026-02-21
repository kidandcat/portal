console.log('app.js loaded');

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

    if (tabName === 'chat') {
        var chat = document.getElementById('chat-messages');
        if (chat) chat.scrollTop = chat.scrollHeight;
    }
}

window.addEventListener('popstate', function() {
    var params = new URLSearchParams(window.location.search);
    var tab = params.get('tab') || 'issues';
    switchTab(tab);
});

document.addEventListener('DOMContentLoaded', function() {
    // Tab click handlers
    document.querySelectorAll('.tab[data-tab]').forEach(function(t) {
        t.addEventListener('click', function(e) {
            e.preventDefault();
            switchTab(this.dataset.tab);
        });
    });

    // Initialize Lucide icons
    if (typeof lucide !== 'undefined') {
        lucide.createIcons();
    }
});

// Re-init Lucide after HTMX swaps
document.addEventListener('htmx:afterSwap', function() {
    if (typeof lucide !== 'undefined') {
        lucide.createIcons();
    }
});
