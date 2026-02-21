// Tab switching
function switchTab(tabName) {
    // Update URL without reload
    var url = new URL(window.location);
    url.searchParams.set('tab', tabName);
    history.pushState({}, '', url);

    // Toggle tab buttons
    document.querySelectorAll('.tab[data-tab]').forEach(function(t) {
        t.classList.toggle('active', t.dataset.tab === tabName);
    });

    // Toggle panels
    document.querySelectorAll('.tab-panel').forEach(function(p) {
        p.classList.toggle('active', p.id === 'tab-' + tabName);
    });

    // Scroll chat to bottom when switching to chat
    if (tabName === 'chat') {
        var chat = document.getElementById('chat-messages');
        if (chat) chat.scrollTop = chat.scrollHeight;
    }
}

// Handle browser back/forward
window.addEventListener('popstate', function() {
    var params = new URLSearchParams(window.location.search);
    var tab = params.get('tab') || 'issues';
    switchTab(tab);
});

// Init tabs on page load
document.addEventListener('DOMContentLoaded', function() {
    // Set up tab click handlers
    document.querySelectorAll('.tab[data-tab]').forEach(function(t) {
        t.addEventListener('click', function(e) {
            e.preventDefault();
            switchTab(this.dataset.tab);
        });
    });
});
