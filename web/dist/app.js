// ============================================
// Napstarr — Vanilla JS SPA
// ============================================

const state = {
    user: null,
    artists: [],
    albums: [],
    currentAlbum: null,
    currentArtist: null,
    queue: [],
    queueIndex: -1,
    playing: false,
    currentTrack: null,
    volume: 0.8,
    searchResults: null,
    stats: null,
};

const audio = new Audio();
audio.volume = state.volume;

// ---- API Client ----

async function api(path, options = {}) {
    const res = await fetch('/api' + path, {
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', ...options.headers },
        ...options,
    });
    if (res.status === 401) {
        state.user = null;
        navigate('/login');
        throw new Error('unauthorized');
    }
    if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || `HTTP ${res.status}`);
    }
    if (res.status === 204 || res.headers.get('content-length') === '0') return null;
    return res.json();
}

// ---- Router ----

function navigate(hash) {
    window.location.hash = hash;
}

function getRoute() {
    const hash = window.location.hash.slice(1) || '/';
    return hash;
}

function matchRoute(pattern, path) {
    const patternParts = pattern.split('/');
    const pathParts = path.split('/');
    if (patternParts.length !== pathParts.length) return null;
    const params = {};
    for (let i = 0; i < patternParts.length; i++) {
        if (patternParts[i].startsWith('{') && patternParts[i].endsWith('}')) {
            params[patternParts[i].slice(1, -1)] = pathParts[i];
        } else if (patternParts[i] !== pathParts[i]) {
            return null;
        }
    }
    return params;
}

async function render() {
    const route = getRoute();
    const app = document.getElementById('app');

    if (route === '/login') {
        app.innerHTML = LoginView();
        bindLoginEvents();
        return;
    }

    if (!state.user) {
        try {
            state.stats = await api('/library/stats');
            state.user = { authenticated: true };
        } catch {
            navigate('/login');
            return;
        }
    }

    let content = '';
    let params;

    if (route === '/') {
        content = await HomeView();
    } else if (route === '/artists') {
        content = await ArtistsView();
    } else if ((params = matchRoute('/artists/{id}', route))) {
        content = await ArtistView(params.id);
    } else if (route === '/albums') {
        content = await AlbumsView();
    } else if ((params = matchRoute('/albums/{id}', route))) {
        content = await AlbumView(params.id);
    } else if (route === '/search') {
        content = await SearchView();
    } else {
        content = '<div class="main-content"><div class="empty-state"><h3>Page Not Found</h3></div></div>';
    }

    app.innerHTML = Sidebar() + content;
    highlightNav();
    bindAppEvents();
}

// ---- Sidebar ----

function Sidebar() {
    return `
    <nav class="sidebar">
        <div class="sidebar-logo">Nap<span>starr</span></div>
        <div class="sidebar-nav">
            <a href="#/" class="nav-link" data-route="/">
                <svg viewBox="0 0 24 24" fill="currentColor"><path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z"/></svg>
                Home
            </a>
            <a href="#/artists" class="nav-link" data-route="/artists">
                <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/></svg>
                Artists
            </a>
            <a href="#/albums" class="nav-link" data-route="/albums">
                <svg viewBox="0 0 24 24" fill="currentColor"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 14.5c-2.49 0-4.5-2.01-4.5-4.5S9.51 7.5 12 7.5s4.5 2.01 4.5 4.5-2.01 4.5-4.5 4.5zm0-5.5c-.55 0-1 .45-1 1s.45 1 1 1 1-.45 1-1-.45-1-1-1z"/></svg>
                Albums
            </a>
            <a href="#/search" class="nav-link" data-route="/search">
                <svg viewBox="0 0 24 24" fill="currentColor"><path d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg>
                Search
            </a>
        </div>
    </nav>`;
}

function highlightNav() {
    const route = getRoute();
    document.querySelectorAll('.nav-link').forEach(link => {
        const r = link.dataset.route;
        if (r === '/' && route === '/') {
            link.classList.add('active');
        } else if (r !== '/' && route.startsWith(r)) {
            link.classList.add('active');
        } else {
            link.classList.remove('active');
        }
    });
}

// ---- Views ----

function LoginView() {
    return `
    <div class="login-page">
        <div class="login-card">
            <h1>Nap<span>starr</span></h1>
            <p>Sign in to your music library</p>
            <form id="login-form">
                <div class="form-group">
                    <label for="username">Username</label>
                    <input type="text" id="username" class="form-input" autocomplete="username" required>
                </div>
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" class="form-input" autocomplete="current-password" required>
                </div>
                <button type="submit" class="btn-login">Sign In</button>
                <div id="login-error" class="login-error"></div>
            </form>
        </div>
    </div>`;
}

function bindLoginEvents() {
    const form = document.getElementById('login-form');
    if (!form) return;
    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        const errEl = document.getElementById('login-error');
        errEl.classList.remove('show');
        try {
            const user = await api('/auth/login', {
                method: 'POST',
                body: JSON.stringify({
                    username: document.getElementById('username').value,
                    password: document.getElementById('password').value,
                }),
            });
            state.user = user;
            navigate('/');
        } catch (err) {
            errEl.textContent = err.message === 'unauthorized' ? 'Invalid credentials' : err.message;
            errEl.classList.add('show');
        }
    });
}

async function HomeView() {
    let albums = [];
    let stats = state.stats;
    try {
        [albums, stats] = await Promise.all([
            api('/albums?recent=true'),
            stats ? Promise.resolve(stats) : api('/library/stats'),
        ]);
        state.stats = stats;
    } catch { /* noop */ }

    return `
    <div class="main-content">
        <div class="page-header">
            <h1>Home</h1>
        </div>
        <div class="stats-grid">
            <div class="stat-card">
                <div class="stat-value">${stats?.artistCount ?? 0}</div>
                <div class="stat-label">Artists</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">${stats?.albumCount ?? 0}</div>
                <div class="stat-label">Albums</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">${stats?.trackCount ?? 0}</div>
                <div class="stat-label">Tracks</div>
            </div>
            <div class="stat-card">
                <div class="stat-value">${stats?.totalSizeMb ? (stats.totalSizeMb / 1024).toFixed(1) + ' GB' : '0 GB'}</div>
                <div class="stat-label">Library Size</div>
            </div>
        </div>
        <div style="margin-bottom:24px">
            <button class="btn-play-all" data-action="scan-library" style="display:inline-flex">
                <svg viewBox="0 0 24 24"><path d="M17.65 6.35C16.2 4.9 14.21 4 12 4c-4.42 0-7.99 3.58-7.99 8s3.57 8 7.99 8c3.73 0 6.84-2.55 7.73-6h-2.08c-.82 2.33-3.04 4-5.65 4-3.31 0-6-2.69-6-6s2.69-6 6-6c1.66 0 3.14.69 4.22 1.78L13 11h7V4l-2.35 2.35z"/></svg>
                Scan Library
            </button>
        </div>
        ${albums.length > 0 ? `
            <h2 class="section-title">Recently Added</h2>
            <div class="album-grid">
                ${albums.map(albumCard).join('')}
            </div>
        ` : `
            <div class="empty-state">
                <svg viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 14.5c-2.49 0-4.5-2.01-4.5-4.5S9.51 7.5 12 7.5s4.5 2.01 4.5 4.5-2.01 4.5-4.5 4.5zm0-5.5c-.55 0-1 .45-1 1s.45 1 1 1 1-.45 1-1-.45-1-1-1z"/></svg>
                <h3>Your library is empty</h3>
                <p>Scan your music folder to get started.</p>
            </div>
        `}
    </div>`;
}

async function ArtistsView() {
    let artists = [];
    try {
        artists = await api('/artists');
        state.artists = artists;
    } catch { /* noop */ }

    return `
    <div class="main-content">
        <div class="page-header">
            <h1>Artists</h1>
            <p>${artists.length} artist${artists.length !== 1 ? 's' : ''}</p>
        </div>
        <div class="artist-grid">
            ${artists.map(a => `
                <div class="artist-card" data-action="go-artist" data-id="${a.id}">
                    <div class="artist-avatar">${getInitials(a.name)}</div>
                    <div class="artist-card-name">${esc(a.name)}</div>
                    <div class="artist-card-albums">${a.albumCount ?? ''} album${(a.albumCount ?? 0) !== 1 ? 's' : ''}</div>
                </div>
            `).join('')}
        </div>
    </div>`;
}

async function ArtistView(id) {
    let artist = null;
    let albums = [];
    try {
        [artist, albums] = await Promise.all([
            api('/artists/' + id),
            api('/artists/' + id + '/albums'),
        ]);
        state.currentArtist = artist;
    } catch {
        return '<div class="main-content"><div class="empty-state"><h3>Artist not found</h3></div></div>';
    }

    return `
    <div class="main-content">
        <div class="artist-header">
            <div class="artist-header-avatar">${getInitials(artist.name)}</div>
            <div class="artist-header-info">
                <h1>${esc(artist.name)}</h1>
                <p>${albums.length} album${albums.length !== 1 ? 's' : ''}</p>
            </div>
        </div>
        <h2 class="section-title">Albums</h2>
        <div class="album-grid">
            ${albums.map(albumCard).join('')}
        </div>
    </div>`;
}

async function AlbumsView() {
    let albums = [];
    try {
        albums = await api('/albums');
        state.albums = albums;
    } catch { /* noop */ }

    return `
    <div class="main-content">
        <div class="page-header">
            <h1>Albums</h1>
            <p>${albums.length} album${albums.length !== 1 ? 's' : ''}</p>
        </div>
        <div class="album-grid">
            ${albums.map(albumCard).join('')}
        </div>
    </div>`;
}

async function AlbumView(id) {
    let album = null;
    try {
        album = await api('/albums/' + id);
        state.currentAlbum = album;
    } catch {
        return '<div class="main-content"><div class="empty-state"><h3>Album not found</h3></div></div>';
    }

    const tracks = album.tracks || [];

    return `
    <div class="main-content">
        <div class="album-header">
            <div class="album-header-art">
                <img src="/api/artwork/album/${album.id}" alt="${esc(album.title)}" loading="lazy">
            </div>
            <div class="album-header-info">
                <h1>${esc(album.title)}</h1>
                <div class="album-meta">
                    <a href="#/artists/${album.artistId}" data-action="go-artist" data-id="${album.artistId}">${esc(album.artistName || 'Unknown Artist')}</a>
                    ${album.year ? ` &middot; ${album.year}` : ''}
                    &middot; ${tracks.length} track${tracks.length !== 1 ? 's' : ''}
                    ${tracks.length > 0 ? ` &middot; ${formatDurationLong(tracks.reduce((s, t) => s + (t.durationMs || 0), 0))}` : ''}
                </div>
                <div class="album-actions">
                    <button class="btn-play-all" data-action="play-album" data-id="${album.id}">
                        <svg viewBox="0 0 24 24"><polygon points="5,3 19,12 5,21"/></svg>
                        Play All
                    </button>
                </div>
            </div>
        </div>
        <div class="track-list">
            <div class="track-list-header">
                <span>#</span>
                <span>Title</span>
                <span>Album</span>
                <span style="text-align:right">Duration</span>
            </div>
            ${tracks.map((t, i) => `
                <div class="track-row${state.currentTrack?.id === t.id ? ' playing' : ''}"
                     data-action="play-track" data-album-id="${album.id}" data-index="${i}">
                    <span class="track-num">${i + 1}</span>
                    <span class="track-title">${esc(t.title)}</span>
                    <span class="track-album">${esc(album.title)}</span>
                    <span class="track-duration">${formatDuration(t.durationMs)}</span>
                </div>
            `).join('')}
        </div>
    </div>`;
}

async function SearchView() {
    return `
    <div class="main-content">
        <div class="page-header">
            <h1>Search</h1>
        </div>
        <div class="search-input-wrapper">
            <svg class="search-icon" viewBox="0 0 24 24" fill="currentColor"><path d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg>
            <input type="text" class="search-input" id="search-input" placeholder="Search artists, albums, tracks..." autofocus>
        </div>
        <div id="search-results"></div>
    </div>`;
}

function renderSearchResults(results) {
    const el = document.getElementById('search-results');
    if (!el) return;

    const { artists = [], albums = [], tracks = [] } = results;
    if (!artists.length && !albums.length && !tracks.length) {
        el.innerHTML = '<div class="empty-state"><h3>No results found</h3></div>';
        return;
    }

    let html = '';

    if (artists.length) {
        html += `<div class="search-section"><h2>Artists</h2><div class="artist-grid">
            ${artists.map(a => `
                <div class="artist-card" data-action="go-artist" data-id="${a.id}">
                    <div class="artist-avatar">${getInitials(a.name)}</div>
                    <div class="artist-card-name">${esc(a.name)}</div>
                </div>
            `).join('')}
        </div></div>`;
    }

    if (albums.length) {
        html += `<div class="search-section"><h2>Albums</h2><div class="album-grid">
            ${albums.map(albumCard).join('')}
        </div></div>`;
    }

    if (tracks.length) {
        html += `<div class="search-section"><h2>Tracks</h2><div class="track-list">
            ${tracks.map((t, i) => `
                <div class="track-row" data-action="play-single" data-track-id="${t.id}">
                    <span class="track-num">${i + 1}</span>
                    <span class="track-title">${esc(t.title)}</span>
                    <span class="track-album">${esc(t.albumTitle || '')}</span>
                    <span class="track-duration">${formatDuration(t.durationMs)}</span>
                </div>
            `).join('')}
        </div></div>`;
    }

    el.innerHTML = html;
}

// ---- Components ----

function albumCard(album) {
    return `
    <div class="album-card" data-action="go-album" data-id="${album.id}">
        <div class="album-card-art">
            <img src="/api/artwork/album/${album.id}" alt="${esc(album.title)}" loading="lazy">
            <div class="play-overlay">
                <div class="play-overlay-btn">
                    <svg viewBox="0 0 24 24"><polygon points="5,3 19,12 5,21"/></svg>
                </div>
            </div>
        </div>
        <div class="album-card-title">${esc(album.title)}</div>
        <div class="album-card-artist">${esc(album.artistName || 'Unknown Artist')}</div>
    </div>`;
}

// ---- Player ----

function initPlayer() {
    const bar = document.getElementById('player-bar');
    bar.innerHTML = `
        <div class="player-progress" id="player-progress">
            <div class="player-progress-fill" id="player-progress-fill"></div>
        </div>
        <div class="player-inner">
            <div class="player-track-info">
                <div class="player-art" id="player-art">
                    <img src="" alt="" id="player-art-img">
                </div>
                <div class="player-track-text">
                    <div class="player-track-title" id="player-title">-</div>
                    <div class="player-track-artist" id="player-artist">-</div>
                </div>
            </div>
            <div class="player-controls">
                <button class="player-btn" id="btn-prev" title="Previous">
                    <svg viewBox="0 0 24 24"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg>
                </button>
                <button class="player-btn player-btn-play" id="btn-play" title="Play">
                    <svg viewBox="0 0 24 24" id="play-icon"><polygon points="5,3 19,12 5,21"/></svg>
                </button>
                <button class="player-btn" id="btn-next" title="Next">
                    <svg viewBox="0 0 24 24"><path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/></svg>
                </button>
            </div>
            <div class="player-time">
                <span id="player-current">0:00</span>
                <span>/</span>
                <span id="player-duration">0:00</span>
            </div>
            <div class="player-volume">
                <svg viewBox="0 0 24 24"><path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02z"/></svg>
                <input type="range" class="volume-slider" id="volume-slider" min="0" max="1" step="0.01" value="${state.volume}">
            </div>
        </div>`;

    // Event listeners
    document.getElementById('btn-play').addEventListener('click', togglePlay);
    document.getElementById('btn-prev').addEventListener('click', prev);
    document.getElementById('btn-next').addEventListener('click', next);

    document.getElementById('volume-slider').addEventListener('input', (e) => {
        state.volume = parseFloat(e.target.value);
        audio.volume = state.volume;
    });

    document.getElementById('player-progress').addEventListener('click', (e) => {
        if (!audio.duration) return;
        const rect = e.currentTarget.getBoundingClientRect();
        const pct = (e.clientX - rect.left) / rect.width;
        audio.currentTime = pct * audio.duration;
    });

    audio.addEventListener('timeupdate', () => {
        if (!audio.duration) return;
        const pct = (audio.currentTime / audio.duration) * 100;
        document.getElementById('player-progress-fill').style.width = pct + '%';
        document.getElementById('player-current').textContent = formatDuration(audio.currentTime * 1000);
        document.getElementById('player-duration').textContent = formatDuration(audio.duration * 1000);
    });

    audio.addEventListener('ended', () => {
        recordListen();
        next();
    });

    audio.addEventListener('play', () => {
        state.playing = true;
        updatePlayButton();
    });

    audio.addEventListener('pause', () => {
        state.playing = false;
        updatePlayButton();
    });
}

function playTrack(track, albumId) {
    state.currentTrack = track;
    audio.src = '/api/stream/' + track.id;
    audio.play();

    const bar = document.getElementById('player-bar');
    bar.classList.add('active');

    document.getElementById('player-title').textContent = track.title;
    document.getElementById('player-artist').textContent = track.artistName || 'Unknown Artist';
    const img = document.getElementById('player-art-img');
    img.src = '/api/artwork/album/' + (albumId || track.albumId || '');
    img.alt = track.title;

    updatePlayButton();
    highlightPlaying();
}

function playAlbum(tracks, startIndex = 0) {
    if (!tracks || !tracks.length) return;
    state.queue = tracks;
    state.queueIndex = startIndex;
    playTrack(tracks[startIndex], state.currentAlbum?.id);
}

function togglePlay() {
    if (!state.currentTrack) return;
    if (audio.paused) {
        audio.play();
    } else {
        audio.pause();
    }
}

function next() {
    if (state.queueIndex < state.queue.length - 1) {
        state.queueIndex++;
        playTrack(state.queue[state.queueIndex], state.currentAlbum?.id);
    }
}

function prev() {
    if (audio.currentTime > 3) {
        audio.currentTime = 0;
        return;
    }
    if (state.queueIndex > 0) {
        state.queueIndex--;
        playTrack(state.queue[state.queueIndex], state.currentAlbum?.id);
    }
}

function updatePlayButton() {
    const icon = document.getElementById('play-icon');
    if (!icon) return;
    if (state.playing) {
        icon.innerHTML = '<rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/>';
    } else {
        icon.innerHTML = '<polygon points="5,3 19,12 5,21"/>';
    }
}

function highlightPlaying() {
    document.querySelectorAll('.track-row').forEach(row => {
        row.classList.remove('playing');
    });
    if (state.currentTrack) {
        document.querySelectorAll('.track-row').forEach(row => {
            const idx = parseInt(row.dataset.index);
            if (!isNaN(idx) && state.queue[idx]?.id === state.currentTrack.id) {
                row.classList.add('playing');
            }
        });
    }
}

function recordListen() {
    if (!state.currentTrack) return;
    const durationMs = Math.round(audio.currentTime * 1000);
    if (durationMs < 5000) return; // only record if listened > 5s
    api('/history', {
        method: 'POST',
        body: JSON.stringify({
            trackId: state.currentTrack.id,
            durationMs,
        }),
    }).catch(() => {});
}

// ---- Event Delegation ----

function bindAppEvents() {
    const app = document.getElementById('app');

    // Debounced search
    const searchInput = document.getElementById('search-input');
    if (searchInput) {
        let searchTimeout;
        searchInput.addEventListener('input', (e) => {
            clearTimeout(searchTimeout);
            const q = e.target.value.trim();
            if (!q) {
                document.getElementById('search-results').innerHTML = '';
                return;
            }
            searchTimeout = setTimeout(async () => {
                try {
                    const results = await api('/search?q=' + encodeURIComponent(q));
                    state.searchResults = results;
                    renderSearchResults(results);
                } catch { /* noop */ }
            }, 300);
        });
    }
}

// Global click delegation
document.addEventListener('click', (e) => {
    const target = e.target.closest('[data-action]');
    if (!target) return;

    const action = target.dataset.action;

    if (action === 'go-album') {
        navigate('/albums/' + target.dataset.id);
    } else if (action === 'go-artist') {
        navigate('/artists/' + target.dataset.id);
    } else if (action === 'play-track') {
        const albumId = target.dataset.albumId;
        const index = parseInt(target.dataset.index);
        if (state.currentAlbum && state.currentAlbum.tracks) {
            playAlbum(state.currentAlbum.tracks, index);
        }
    } else if (action === 'play-album') {
        if (state.currentAlbum && state.currentAlbum.tracks) {
            playAlbum(state.currentAlbum.tracks, 0);
        }
    } else if (action === 'scan-library') {
        api('/library/scan', { method: 'POST' }).then(() => {
            target.textContent = 'Scanning...';
            target.disabled = true;
            setTimeout(() => { render(); }, 10000);
        }).catch(() => {});
    } else if (action === 'play-single') {
        const trackId = target.dataset.trackId;
        api('/tracks/' + trackId).then(track => {
            state.queue = [track];
            state.queueIndex = 0;
            playTrack(track, track.albumId);
        }).catch(() => {});
    }
});

// ---- Keyboard Shortcuts ----

document.addEventListener('keydown', (e) => {
    // Don't intercept when typing in inputs
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
        if (e.key === 'Escape') e.target.blur();
        return;
    }

    switch (e.key) {
        case ' ':
            e.preventDefault();
            togglePlay();
            break;
        case 'ArrowRight':
            if (audio.duration) audio.currentTime = Math.min(audio.duration, audio.currentTime + 10);
            break;
        case 'ArrowLeft':
            audio.currentTime = Math.max(0, audio.currentTime - 10);
            break;
        case 'ArrowUp':
            e.preventDefault();
            state.volume = Math.min(1, state.volume + 0.05);
            audio.volume = state.volume;
            updateVolumeSlider();
            break;
        case 'ArrowDown':
            e.preventDefault();
            state.volume = Math.max(0, state.volume - 0.05);
            audio.volume = state.volume;
            updateVolumeSlider();
            break;
    }
});

function updateVolumeSlider() {
    const slider = document.getElementById('volume-slider');
    if (slider) slider.value = state.volume;
}

// ---- Helpers ----

function formatDuration(ms) {
    if (!ms || ms <= 0) return '0:00';
    const totalSeconds = Math.floor(ms / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}

function formatDurationLong(ms) {
    if (!ms || ms <= 0) return '0 min';
    const totalMinutes = Math.floor(ms / 60000);
    if (totalMinutes < 60) return `${totalMinutes} min`;
    const hours = Math.floor(totalMinutes / 60);
    const mins = totalMinutes % 60;
    return `${hours} hr ${mins} min`;
}

function formatDurationHours(ms) {
    if (!ms || ms <= 0) return '0h';
    const hours = Math.floor(ms / 3600000);
    if (hours > 0) return `${hours}h`;
    const minutes = Math.floor(ms / 60000);
    return `${minutes}m`;
}

function esc(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function getInitials(name) {
    if (!name) return '?';
    return name.split(' ').slice(0, 2).map(w => w[0]).join('').toUpperCase();
}

// ---- Init ----

window.addEventListener('hashchange', render);

initPlayer();
render();
