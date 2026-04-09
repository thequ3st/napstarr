// ============================================
// Napstarr — Vanilla JS SPA
// ============================================

(function () {
  'use strict';

  // ---- State ----
  const state = {
    authenticated: false,
    view: 'home',
    viewParams: {},
    instance: null,
    stats: null,
    peers: [],
    queue: [],
    queueIndex: -1,
    playing: false,
    currentTrack: null,
    currentAlbumId: null,
    volume: 0.8,
    isRemote: false,
    remotePeerId: null,
    remotePeerName: null,
  };

  const audio = document.getElementById('audio-el');
  audio.volume = state.volume;

  // ---- Refs ----
  const $login = document.getElementById('login-screen');
  const $app = document.getElementById('app-shell');
  const $main = document.getElementById('main-content');
  const $modal = document.getElementById('modal-overlay');
  const $modalContent = document.getElementById('modal-content');

  // ---- API ----
  async function api(path, opts = {}) {
    const res = await fetch('/api' + path, {
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', ...opts.headers },
      ...opts,
    });
    if (res.status === 401) {
      state.authenticated = false;
      showLogin();
      throw new Error('unauthorized');
    }
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      throw new Error(d.error || 'HTTP ' + res.status);
    }
    if (res.status === 204 || res.headers.get('content-length') === '0') return null;
    return res.json();
  }

  // ---- HTML Helpers ----
  function esc(str) {
    if (!str) return '';
    const d = document.createElement('div');
    d.textContent = String(str);
    return d.innerHTML;
  }

  function initials(name) {
    if (!name) return '?';
    return name.split(/\s+/).slice(0, 2).map(w => w.charAt(0)).join('').toUpperCase();
  }

  function fmtDuration(ms) {
    if (!ms || ms <= 0) return '0:00';
    const total = Math.floor(ms / 1000);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return m + ':' + String(s).padStart(2, '0');
  }

  function fmtDurationLong(ms) {
    if (!ms || ms <= 0) return '0 min';
    const totalMin = Math.floor(ms / 60000);
    if (totalMin < 60) return totalMin + ' min';
    return Math.floor(totalMin / 60) + ' hr ' + (totalMin % 60) + ' min';
  }

  function fmtSize(mb) {
    if (!mb) return '0 MB';
    if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB';
    return Math.round(mb) + ' MB';
  }

  function truncate(str, len) {
    if (!str) return '';
    return str.length > len ? str.slice(0, len) + '...' : str;
  }

  function timeAgo(dateStr) {
    if (!dateStr) return 'never';
    const diff = Date.now() - new Date(dateStr).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return 'just now';
    if (mins < 60) return mins + 'm ago';
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return hrs + 'h ago';
    const days = Math.floor(hrs / 24);
    return days + 'd ago';
  }

  function loading() {
    return '<div class="loading"><div class="spinner"></div></div>';
  }

  function emptyState(icon, title, desc) {
    return `<div class="empty-state">
      <div class="empty-state-icon">${icon}</div>
      <h3>${esc(title)}</h3>
      <p>${esc(desc)}</p>
    </div>`;
  }

  const svgDisc = '<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="1.5"/><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>';
  const svgPlay = '<polygon points="8,5 19,12 8,19" fill="currentColor"/>';
  const svgShuffle = '<path d="M16 3h5v5M4 20L20.2 3.8M21 16v5h-5M15 15l5.1 5.1M4 4l5 5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const svgBack = '<path d="M19 12H5m0 0l7 7m-7-7l7-7" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const svgSearch = '<path d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const svgGlobe = '<path d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const svgRefresh = '<path d="M4 4v5h5M20 20v-5h-5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/><path d="M20.49 9A9 9 0 005.64 5.64L4 4m16 16l-1.64-1.64A9 9 0 013.51 15" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const svgUser = '<path d="M16 7a4 4 0 11-8 0 4 4 0 018 0zm-4 7a7 7 0 00-7 7h14a7 7 0 00-7-7z" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';
  const svgX = '<path d="M18 6L6 18M6 6l12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>';
  const svgTrash = '<path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m3 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6h14" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>';

  // ---- Toast ----
  function toast(msg) {
    const el = document.createElement('div');
    el.className = 'toast';
    el.textContent = msg;
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 3200);
  }

  // ---- Auth ----
  function showLogin() {
    $login.classList.remove('hidden');
    $app.classList.add('hidden');
  }

  function showApp() {
    $login.classList.add('hidden');
    $app.classList.remove('hidden');
  }

  async function checkAuth() {
    try {
      state.stats = await api('/library/stats');
      state.authenticated = true;
      showApp();
      loadInstance();
      navigateTo(state.view, state.viewParams);
    } catch {
      showLogin();
    }
  }

  // Login form
  document.getElementById('login-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const errEl = document.getElementById('login-error');
    errEl.textContent = '';
    const user = document.getElementById('login-user').value;
    const pass = document.getElementById('login-pass').value;
    try {
      await api('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username: user, password: pass }),
      });
      state.authenticated = true;
      showApp();
      loadInstance();
      navigateTo('home');
    } catch (err) {
      errEl.textContent = err.message === 'unauthorized' ? 'Invalid credentials' : err.message;
    }
  });

  // ---- Instance Info ----
  async function loadInstance() {
    try {
      state.instance = await api('/instance');
      const nameEl = document.getElementById('instance-name');
      const idEl = document.getElementById('instance-id');
      if (nameEl) nameEl.textContent = state.instance.name || 'Napstarr';
      if (idEl) idEl.textContent = truncate(state.instance.id, 12);
    } catch { /* ok */ }
  }

  // ---- Navigation ----
  function navigateTo(view, params) {
    state.view = view;
    state.viewParams = params || {};
    updateNav();
    renderView();
  }

  function updateNav() {
    // Sidebar
    document.querySelectorAll('#sidebar-nav .nav-link').forEach(el => {
      el.classList.toggle('active', el.dataset.view === state.view);
    });
    // Mobile
    document.querySelectorAll('#mobile-tabs .tab-link').forEach(el => {
      el.classList.toggle('active', el.dataset.view === state.view);
    });
  }

  // Nav click handlers
  document.querySelectorAll('[data-view]').forEach(el => {
    el.addEventListener('click', (e) => {
      e.preventDefault();
      navigateTo(el.dataset.view);
    });
  });

  // ---- View Router ----
  async function renderView() {
    $main.innerHTML = loading();
    try {
      switch (state.view) {
        case 'home': await renderHome(); break;
        case 'artists': await renderArtists(); break;
        case 'artist-detail': await renderArtistDetail(state.viewParams.id); break;
        case 'albums': await renderAlbums(); break;
        case 'album-detail': await renderAlbumDetail(state.viewParams.id); break;
        case 'search': renderSearch(); break;
        case 'network': await renderNetwork(); break;
        default: $main.innerHTML = emptyState(svgDisc, 'Page Not Found', '');
      }
    } catch (err) {
      console.error('View error:', err);
      $main.innerHTML = emptyState(svgDisc, 'Something went wrong', err.message);
    }
  }

  // ---- Home View ----
  async function renderHome() {
    const [stats, recentAlbums, peers] = await Promise.all([
      api('/library/stats').catch(() => state.stats),
      api('/albums?recent=true').catch(() => []),
      api('/peers').catch(() => []),
    ]);
    state.stats = stats || {};
    state.peers = peers || [];
    const s = state.stats;
    const albums = recentAlbums || [];

    let html = `
      <div class="page-header">
        <h1>Home</h1>
        ${state.instance ? `<p class="home-welcome">Welcome to <strong>${esc(state.instance.name)}</strong></p>` : ''}
      </div>
      <div class="stats-grid">
        <div class="stat-card"><div class="stat-value">${s.artistCount || 0}</div><div class="stat-label">Artists</div></div>
        <div class="stat-card"><div class="stat-value">${s.albumCount || 0}</div><div class="stat-label">Albums</div></div>
        <div class="stat-card"><div class="stat-value">${s.trackCount || 0}</div><div class="stat-label">Tracks</div></div>
        <div class="stat-card"><div class="stat-value">${fmtSize(s.totalSizeMb)}</div><div class="stat-label">Library Size</div></div>
      </div>
      <div class="scan-row">
        <button class="btn btn-secondary btn-sm" id="btn-scan">
          <svg viewBox="0 0 24 24" width="16" height="16">${svgRefresh}</svg>
          Scan Library
        </button>
        <span class="scan-status" id="scan-status"></span>
      </div>`;

    if (albums.length > 0) {
      html += `<h2 class="section-title">Recently Added</h2>
        <div class="album-grid">${albums.map(albumCard).join('')}</div>`;
    } else {
      html += emptyState(svgDisc, 'Your library is empty', 'Scan your music folder to get started.');
    }

    if (state.peers.length > 0) {
      html += `<div style="margin-top:40px">
        <h2 class="section-title">Network Activity</h2>
        <div class="network-peers-row">
          ${state.peers.map(p => `
            <div class="network-peer-chip" data-action="go-network">
              <span class="peer-dot ${p.status === 'online' ? 'online' : 'offline'}"></span>
              <span>${esc(p.name || truncate(p.instanceId, 8))}</span>
            </div>
          `).join('')}
        </div>
      </div>`;
    }

    $main.innerHTML = html;

    // Scan button
    const scanBtn = document.getElementById('btn-scan');
    if (scanBtn) {
      scanBtn.addEventListener('click', async () => {
        scanBtn.disabled = true;
        document.getElementById('scan-status').textContent = 'Scanning...';
        try {
          await api('/library/scan', { method: 'POST' });
          toast('Library scan started');
          setTimeout(() => {
            document.getElementById('scan-status').textContent = '';
            scanBtn.disabled = false;
            renderHome();
          }, 8000);
        } catch (err) {
          document.getElementById('scan-status').textContent = 'Error: ' + err.message;
          scanBtn.disabled = false;
        }
      });
    }
  }

  // ---- Artists View ----
  async function renderArtists() {
    const artists = (await api('/artists')) || [];

    if (!artists.length) {
      $main.innerHTML = `<div class="page-header"><h1>Artists</h1></div>` +
        emptyState(`<svg viewBox="0 0 24 24" width="56" height="56">${svgUser}</svg>`, 'No artists yet', 'Add music to your library to see artists here.');
      return;
    }

    $main.innerHTML = `
      <div class="page-header">
        <h1>Artists</h1>
        <p>${artists.length} artist${artists.length !== 1 ? 's' : ''}</p>
      </div>
      <div class="artist-grid">
        ${artists.map(a => `
          <div class="artist-card" data-action="go-artist" data-id="${a.id}">
            <div class="artist-avatar">${initials(a.name)}</div>
            <div class="artist-card-name">${esc(a.name)}</div>
            <div class="artist-card-meta">${a.albumCount || 0} album${(a.albumCount || 0) !== 1 ? 's' : ''}${a.trackCount ? ' &middot; ' + a.trackCount + ' tracks' : ''}</div>
          </div>
        `).join('')}
      </div>`;
  }

  // ---- Artist Detail ----
  async function renderArtistDetail(id) {
    const [artist, albums] = await Promise.all([
      api('/artists/' + id),
      api('/artists/' + id + '/albums'),
    ]);

    if (!artist) {
      $main.innerHTML = emptyState(`<svg viewBox="0 0 24 24" width="56" height="56">${svgUser}</svg>`, 'Artist not found', '');
      return;
    }

    const albumList = albums || [];

    $main.innerHTML = `
      <a href="#" class="back-link" data-action="go-artists"><svg viewBox="0 0 24 24" width="16" height="16">${svgBack}</svg> Artists</a>
      <div class="artist-detail-header">
        <div class="artist-detail-avatar">${initials(artist.name)}</div>
        <div class="artist-detail-info">
          <h1>${esc(artist.name)}</h1>
          <p>${albumList.length} album${albumList.length !== 1 ? 's' : ''}${artist.trackCount ? ' &middot; ' + artist.trackCount + ' tracks' : ''}</p>
        </div>
      </div>
      <h2 class="section-title">Albums</h2>
      <div class="album-grid">${albumList.map(albumCard).join('')}</div>`;
  }

  // ---- Albums View ----
  async function renderAlbums() {
    const albums = (await api('/albums')) || [];

    if (!albums.length) {
      $main.innerHTML = `<div class="page-header"><h1>Albums</h1></div>` +
        emptyState(svgDisc, 'No albums yet', 'Add music to your library to see albums here.');
      return;
    }

    $main.innerHTML = `
      <div class="page-header">
        <h1>Albums</h1>
        <p>${albums.length} album${albums.length !== 1 ? 's' : ''}</p>
      </div>
      <div class="album-grid">${albums.map(albumCard).join('')}</div>`;
  }

  // ---- Album Detail ----
  async function renderAlbumDetail(id) {
    const album = await api('/albums/' + id);
    if (!album) {
      $main.innerHTML = emptyState(svgDisc, 'Album not found', '');
      return;
    }

    state.currentAlbumId = album.id;
    const tracks = album.tracks || [];
    const totalMs = tracks.reduce((sum, t) => sum + (t.durationMs || 0), 0);

    $main.innerHTML = `
      <a href="#" class="back-link" data-action="go-albums"><svg viewBox="0 0 24 24" width="16" height="16">${svgBack}</svg> Albums</a>
      <div class="album-detail-header">
        <div class="album-detail-art">
          <img src="/api/artwork/album/${album.id}" alt="${esc(album.title)}" loading="lazy">
        </div>
        <div class="album-detail-info">
          <div class="album-detail-type">Album</div>
          <h1>${esc(album.title)}</h1>
          <div class="album-detail-meta">
            <a href="#" data-action="go-artist" data-id="${album.artistId}">${esc(album.artistName || 'Unknown Artist')}</a>
            ${album.year ? `<span class="dot">&middot;</span><span>${album.year}</span>` : ''}
            <span class="dot">&middot;</span>
            <span>${tracks.length} track${tracks.length !== 1 ? 's' : ''}</span>
            ${totalMs > 0 ? `<span class="dot">&middot;</span><span>${fmtDurationLong(totalMs)}</span>` : ''}
          </div>
          <div class="album-actions">
            <button class="btn btn-primary btn-pill" data-action="play-album" data-id="${album.id}">
              <svg viewBox="0 0 24 24" width="18" height="18">${svgPlay}</svg> Play All
            </button>
            <button class="btn btn-secondary btn-pill" data-action="shuffle-album" data-id="${album.id}">
              <svg viewBox="0 0 24 24" width="18" height="18">${svgShuffle}</svg> Shuffle
            </button>
          </div>
        </div>
      </div>
      <div class="track-list">
        <div class="track-list-header">
          <span>#</span><span>Title</span><span>Album</span><span style="text-align:right">Duration</span>
        </div>
        ${tracks.map((t, i) => trackRow(t, i, album)).join('')}
      </div>`;

    highlightPlayingTrack();
  }

  // ---- Search View ----
  function renderSearch() {
    $main.innerHTML = `
      <div class="page-header"><h1>Search</h1></div>
      <div class="search-input-wrapper">
        <svg class="search-icon" viewBox="0 0 24 24">${svgSearch}</svg>
        <input type="text" class="search-input" id="search-input" placeholder="Search tracks..." autofocus>
      </div>
      <div id="search-results"></div>`;

    let timer;
    const input = document.getElementById('search-input');
    input.addEventListener('input', () => {
      clearTimeout(timer);
      const q = input.value.trim();
      if (!q) {
        document.getElementById('search-results').innerHTML = '';
        return;
      }
      timer = setTimeout(async () => {
        try {
          const results = (await api('/search?q=' + encodeURIComponent(q))) || [];
          renderSearchResults(results);
        } catch { /* ok */ }
      }, 300);
    });
  }

  function renderSearchResults(tracks) {
    const el = document.getElementById('search-results');
    if (!el) return;

    if (!tracks.length) {
      el.innerHTML = emptyState(`<svg viewBox="0 0 24 24" width="56" height="56">${svgSearch}</svg>`, 'No results found', 'Try a different search term.');
      return;
    }

    el.innerHTML = `
      <div class="search-section">
        <h2>Tracks</h2>
        <div class="track-list">
          ${tracks.map((t, i) => `
            <div class="track-row${state.currentTrack?.id === t.id ? ' playing' : ''}"
                 data-action="play-search-track" data-track-id="${t.id}" data-index="${i}">
              <span class="track-num">
                <span class="num-text">${i + 1}</span>
                <div class="playing-indicator"><span></span><span></span><span></span></div>
              </span>
              <span class="track-title">${esc(t.title)}</span>
              <span class="track-album-col">${esc(t.artistName || '')}${t.albumTitle ? ' &middot; ' + esc(t.albumTitle) : ''}</span>
              <span class="track-duration">${fmtDuration(t.durationMs)}</span>
            </div>
          `).join('')}
        </div>
      </div>`;

    // Store search tracks for queue building
    el._searchTracks = tracks;
  }

  // ---- Network View ----
  async function renderNetwork() {
    const [instance, peers] = await Promise.all([
      state.instance || api('/instance').catch(() => null),
      api('/peers').catch(() => []),
    ]);
    state.instance = instance;
    state.peers = peers || [];

    let html = `<div class="page-header"><h1>Network</h1><p>P2P Federation</p></div>`;

    // Instance card
    if (instance) {
      html += `<div class="instance-card">
        <h2>Your Instance</h2>
        <div class="instance-detail-row">
          <span class="instance-detail-label">Name</span>
          <span class="instance-detail-value">${esc(instance.name)}</span>
        </div>
        <div class="instance-detail-row">
          <span class="instance-detail-label">Instance ID</span>
          <span class="instance-detail-value">${esc(instance.id)}</span>
        </div>
        ${instance.publicKey ? `<div class="instance-detail-row">
          <span class="instance-detail-label">Public Key</span>
          <span class="instance-detail-value">${esc(truncate(instance.publicKey, 40))}</span>
        </div>` : ''}
        ${instance.version ? `<div class="instance-detail-row">
          <span class="instance-detail-label">Version</span>
          <span class="instance-detail-value">${esc(instance.version)}</span>
        </div>` : ''}
        ${instance.protocol ? `<div class="instance-detail-row">
          <span class="instance-detail-label">Protocol</span>
          <span class="instance-detail-value">${esc(instance.protocol)}</span>
        </div>` : ''}
        ${instance.stats ? `<div class="instance-detail-row">
          <span class="instance-detail-label">Library</span>
          <span class="instance-detail-value">${instance.stats.artistCount || 0} artists, ${instance.stats.albumCount || 0} albums, ${instance.stats.trackCount || 0} tracks</span>
        </div>` : ''}
      </div>`;
    }

    // Follow new peer
    html += `<div class="network-section">
      <h2 class="section-title">Follow New Peer</h2>
      <div class="follow-form">
        <input type="text" class="follow-input" id="follow-input" placeholder="http://friend:8484">
        <button class="btn btn-primary btn-sm" id="btn-follow">Follow</button>
      </div>
    </div>`;

    // Peers list
    html += `<div class="network-section">
      <div class="section-header">
        <h2 class="section-title">Followed Peers</h2>
        <span style="font-size:13px;color:var(--text-secondary)">${state.peers.length} peer${state.peers.length !== 1 ? 's' : ''}</span>
      </div>`;

    if (state.peers.length === 0) {
      html += emptyState(`<svg viewBox="0 0 24 24" width="56" height="56">${svgGlobe}</svg>`, 'No peers yet', 'Follow another Napstarr instance to explore their library.');
    } else {
      html += `<div class="peer-list">
        ${state.peers.map(p => `
          <div class="peer-card" data-action="view-peer" data-peer-id="${p.id}" data-peer-name="${esc(p.name || '')}">
            <span class="peer-dot ${p.status === 'online' ? 'online' : 'offline'}"></span>
            <div class="peer-info">
              <div class="peer-name">${esc(p.name || 'Unknown Peer')}</div>
              <div class="peer-meta">
                <span>ID: ${esc(truncate(p.instanceId, 10))}</span>
                <span>Last synced: ${timeAgo(p.lastSynced)}</span>
                ${p.lastSeen ? `<span>Seen: ${timeAgo(p.lastSeen)}</span>` : ''}
              </div>
            </div>
            <div class="peer-actions">
              <button class="btn btn-ghost btn-sm" data-action="sync-peer" data-peer-id="${p.id}" title="Sync">
                <svg viewBox="0 0 24 24" width="16" height="16">${svgRefresh}</svg>
              </button>
              <button class="btn btn-ghost btn-sm" data-action="unfollow-peer" data-peer-id="${p.id}" title="Unfollow">
                <svg viewBox="0 0 24 24" width="16" height="16">${svgTrash}</svg>
              </button>
            </div>
          </div>
        `).join('')}
      </div>`;
    }

    html += '</div>';
    $main.innerHTML = html;

    // Follow button
    document.getElementById('btn-follow').addEventListener('click', async () => {
      const input = document.getElementById('follow-input');
      const addr = input.value.trim();
      if (!addr) return;
      try {
        await api('/peers', { method: 'POST', body: JSON.stringify({ address: addr }) });
        input.value = '';
        toast('Peer followed successfully');
        renderNetwork();
      } catch (err) {
        toast('Error: ' + err.message);
      }
    });

    // Enter to follow
    document.getElementById('follow-input').addEventListener('keydown', (e) => {
      if (e.key === 'Enter') document.getElementById('btn-follow').click();
    });
  }

  // ---- Peer Library Modal ----
  async function showPeerLibrary(peerId, peerName) {
    $modal.classList.remove('hidden');
    $modalContent.innerHTML = `
      <button class="modal-close" data-action="close-modal"><svg viewBox="0 0 24 24" width="18" height="18">${svgX}</svg></button>
      <div class="modal-header">
        <h2>${esc(peerName || 'Peer Library')}</h2>
        <p>Browse and stream from this peer</p>
      </div>
      ${loading()}`;

    try {
      const [artists, albums] = await Promise.all([
        api('/peers/' + peerId + '/artists').catch(() => []),
        api('/peers/' + peerId + '/albums').catch(() => []),
      ]);

      const artistList = artists || [];
      const albumList = albums || [];

      let content = `
        <button class="modal-close" data-action="close-modal"><svg viewBox="0 0 24 24" width="18" height="18">${svgX}</svg></button>
        <div class="modal-header">
          <h2>${esc(peerName || 'Peer Library')}</h2>
          <p>${artistList.length} artists, ${albumList.length} albums</p>
        </div>`;

      if (artistList.length > 0) {
        content += `<h3 class="section-title" style="font-size:16px">Artists</h3>
          <div class="artist-grid" style="margin-bottom:28px">
            ${artistList.slice(0, 12).map(a => `
              <div class="artist-card" style="cursor:default">
                <div class="artist-avatar" style="width:80px;height:80px;font-size:24px">${initials(a.name)}</div>
                <div class="artist-card-name">${esc(a.name)}</div>
                <div class="artist-card-meta">${a.albumCount || 0} albums</div>
              </div>
            `).join('')}
            ${artistList.length > 12 ? `<div class="artist-card" style="cursor:default;opacity:0.5"><div class="artist-avatar" style="width:80px;height:80px;font-size:16px">+${artistList.length - 12}</div><div class="artist-card-name">more</div></div>` : ''}
          </div>`;
      }

      if (albumList.length > 0) {
        content += `<h3 class="section-title" style="font-size:16px">Albums</h3>
          <div class="album-grid">
            ${albumList.map(a => `
              <div class="album-card" data-action="view-remote-album" data-peer-id="${peerId}" data-album-id="${a.id}" data-peer-name="${esc(peerName || '')}">
                <div class="album-card-art">
                  <img src="/api/artwork/album/${a.id}" alt="${esc(a.title)}" loading="lazy" onerror="this.style.display='none'">
                  <div class="play-overlay">
                    <div class="play-overlay-btn"><svg viewBox="0 0 24 24">${svgPlay}</svg></div>
                  </div>
                </div>
                <div class="album-card-title">${esc(a.title)}</div>
                <div class="album-card-artist">${esc(a.artistName || 'Unknown')}</div>
                ${a.year ? `<div class="album-card-year">${a.year}</div>` : ''}
              </div>
            `).join('')}
          </div>`;
      }

      if (!artistList.length && !albumList.length) {
        content += emptyState(svgDisc, 'Empty library', 'This peer has no music shared yet.');
      }

      $modalContent.innerHTML = content;
    } catch (err) {
      $modalContent.innerHTML = `
        <button class="modal-close" data-action="close-modal"><svg viewBox="0 0 24 24" width="18" height="18">${svgX}</svg></button>
        ${emptyState(svgGlobe, 'Could not load peer library', err.message)}`;
    }
  }

  // ---- Remote Album Detail in Modal ----
  async function showRemoteAlbum(peerId, albumId, peerName) {
    $modalContent.innerHTML = loading();
    try {
      // We get albums list from peer; individual album detail may not be available for remote
      // Use peer's albums list to find the album info
      const albums = (await api('/peers/' + peerId + '/albums')) || [];
      const album = albums.find(a => a.id === albumId);

      if (!album) {
        $modalContent.innerHTML = `
          <button class="modal-close" data-action="close-modal"><svg viewBox="0 0 24 24" width="18" height="18">${svgX}</svg></button>
          ${emptyState(svgDisc, 'Album not found', '')}`;
        return;
      }

      let content = `
        <button class="modal-close" data-action="close-modal"><svg viewBox="0 0 24 24" width="18" height="18">${svgX}</svg></button>
        <div style="display:flex;gap:20px;align-items:flex-end;margin-bottom:24px">
          <div style="width:120px;height:120px;border-radius:8px;overflow:hidden;background:var(--bg-hover);flex-shrink:0">
            <img src="/api/artwork/album/${album.id}" style="width:100%;height:100%;object-fit:cover" onerror="this.style.display='none'">
          </div>
          <div>
            <div style="font-size:11px;text-transform:uppercase;letter-spacing:0.8px;color:var(--text-muted);margin-bottom:4px">Remote Album</div>
            <h2 style="font-size:22px;font-weight:700">${esc(album.title)}</h2>
            <p style="color:var(--text-secondary);font-size:13px;margin-top:4px">${esc(album.artistName || 'Unknown')}${album.year ? ' &middot; ' + album.year : ''} &middot; ${album.trackCount || '?'} tracks</p>
            <p style="color:var(--accent);font-size:12px;margin-top:6px">Streaming from ${esc(peerName || 'peer')}</p>
          </div>
        </div>
        <p style="color:var(--text-secondary);font-size:13px">Track-level streaming from remote peers requires selecting tracks from the peer album view. Browse the peer library to stream individual tracks.</p>`;

      $modalContent.innerHTML = content;
    } catch (err) {
      $modalContent.innerHTML = `
        <button class="modal-close" data-action="close-modal"><svg viewBox="0 0 24 24" width="18" height="18">${svgX}</svg></button>
        ${emptyState(svgDisc, 'Error loading album', err.message)}`;
    }
  }

  function closeModal() {
    $modal.classList.add('hidden');
    $modalContent.innerHTML = '';
  }

  // ---- Components ----
  function albumCard(album) {
    return `
      <div class="album-card" data-action="go-album" data-id="${album.id}">
        <div class="album-card-art">
          <img src="/api/artwork/album/${album.id}" alt="${esc(album.title)}" loading="lazy">
          <div class="play-overlay">
            <div class="play-overlay-btn"><svg viewBox="0 0 24 24">${svgPlay}</svg></div>
          </div>
        </div>
        <div class="album-card-title">${esc(album.title)}</div>
        <div class="album-card-artist">${esc(album.artistName || 'Unknown Artist')}</div>
        ${album.year ? `<div class="album-card-year">${album.year}</div>` : ''}
      </div>`;
  }

  function trackRow(track, index, album) {
    const isPlaying = state.currentTrack && state.currentTrack.id === track.id;
    return `
      <div class="track-row${isPlaying ? ' playing' : ''}"
           data-action="play-album-track" data-album-id="${album.id}" data-index="${index}">
        <span class="track-num">
          <span class="num-text">${track.trackNumber || index + 1}</span>
          <div class="playing-indicator"><span></span><span></span><span></span></div>
        </span>
        <span class="track-title">${esc(track.title)}</span>
        <span class="track-album-col">${esc(album.title)}</span>
        <span class="track-duration">${fmtDuration(track.durationMs)}</span>
      </div>`;
  }

  function highlightPlayingTrack() {
    document.querySelectorAll('.track-row').forEach(row => {
      const idx = parseInt(row.dataset.index);
      const isPlaying = !isNaN(idx) && state.queue[idx] &&
        state.currentTrack && state.queue[idx].track.id === state.currentTrack.id;
      row.classList.toggle('playing', isPlaying);
    });
  }

  // ---- Player ----
  function playTrack(track, albumId, isRemote, peerId, peerName) {
    state.currentTrack = track;
    state.isRemote = !!isRemote;
    state.remotePeerId = peerId || null;
    state.remotePeerName = peerName || null;

    if (isRemote && peerId) {
      audio.src = '/api/stream/remote/' + peerId + '/' + track.id;
    } else {
      audio.src = '/api/stream/' + track.id;
    }
    audio.play().catch(err => {
      console.warn('Play failed:', err);
    });

    // Update player UI
    document.getElementById('player-title').textContent = track.title || 'Unknown';
    document.getElementById('player-artist').textContent = track.artistName || 'Unknown Artist';

    // Album art
    const artEl = document.getElementById('player-art');
    const artAlbumId = albumId || track.albumId;
    if (artAlbumId) {
      artEl.innerHTML = `<img src="/api/artwork/album/${artAlbumId}" alt="" loading="lazy">`;
    }

    // Remote badge
    const badge = document.getElementById('player-remote-badge');
    const nameSpan = document.getElementById('player-remote-name');
    if (isRemote && peerName) {
      badge.classList.remove('hidden');
      nameSpan.textContent = peerName;
    } else {
      badge.classList.add('hidden');
    }

    highlightPlayingTrack();
  }

  function playQueue(queueItems, startIndex) {
    if (!queueItems || !queueItems.length) return;
    state.queue = queueItems;
    state.queueIndex = startIndex || 0;
    const item = state.queue[state.queueIndex];
    playTrack(item.track, item.albumId, item.isRemote, item.peerId, item.peerName);
  }

  function togglePlay() {
    if (!state.currentTrack) return;
    if (audio.paused) audio.play().catch(() => {});
    else audio.pause();
  }

  function nextTrack() {
    if (state.queueIndex < state.queue.length - 1) {
      state.queueIndex++;
      const item = state.queue[state.queueIndex];
      playTrack(item.track, item.albumId, item.isRemote, item.peerId, item.peerName);
    }
  }

  function prevTrack() {
    if (audio.currentTime > 3) {
      audio.currentTime = 0;
      return;
    }
    if (state.queueIndex > 0) {
      state.queueIndex--;
      const item = state.queue[state.queueIndex];
      playTrack(item.track, item.albumId, item.isRemote, item.peerId, item.peerName);
    }
  }

  function updatePlayIcon() {
    const icon = document.getElementById('play-icon');
    if (!icon) return;
    if (state.playing) {
      icon.innerHTML = '<rect x="7" y="5" width="4" height="14" rx="1" fill="currentColor"/><rect x="13" y="5" width="4" height="14" rx="1" fill="currentColor"/>';
    } else {
      icon.innerHTML = svgPlay;
    }
  }

  function recordListen() {
    if (!state.currentTrack || state.isRemote) return;
    const ms = Math.round(audio.currentTime * 1000);
    if (ms < 5000) return;
    api('/history', {
      method: 'POST',
      body: JSON.stringify({ trackId: state.currentTrack.id, durationMs: ms }),
    }).catch(() => {});
  }

  // Player event listeners
  document.getElementById('btn-play').addEventListener('click', togglePlay);
  document.getElementById('btn-prev').addEventListener('click', prevTrack);
  document.getElementById('btn-next').addEventListener('click', nextTrack);

  document.getElementById('volume-slider').addEventListener('input', (e) => {
    state.volume = parseFloat(e.target.value);
    audio.volume = state.volume;
  });

  document.getElementById('player-progress').addEventListener('click', (e) => {
    if (!audio.duration) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    audio.currentTime = Math.max(0, Math.min(audio.duration, pct * audio.duration));
  });

  audio.addEventListener('timeupdate', () => {
    if (!audio.duration) return;
    const pct = (audio.currentTime / audio.duration) * 100;
    document.getElementById('player-progress-fill').style.width = pct + '%';
    document.getElementById('player-current').textContent = fmtDuration(audio.currentTime * 1000);
    document.getElementById('player-duration').textContent = fmtDuration(audio.duration * 1000);
  });

  audio.addEventListener('play', () => { state.playing = true; updatePlayIcon(); });
  audio.addEventListener('pause', () => { state.playing = false; updatePlayIcon(); });

  audio.addEventListener('ended', () => {
    recordListen();
    nextTrack();
  });

  // Handle stream errors — track file not accessible, skip to next
  audio.addEventListener('error', () => {
    console.warn('Stream error for track:', state.currentTrack?.title);
    const titleEl = document.getElementById('player-title');
    if (titleEl && state.currentTrack) {
      titleEl.textContent = state.currentTrack.title + ' — unavailable';
      titleEl.style.color = '#ef4444';
      setTimeout(() => { titleEl.style.color = ''; }, 2000);
    }
    // Auto-skip to next track quickly
    setTimeout(() => nextTrack(), 800);
  });

  // Show loading state while audio is buffering
  audio.addEventListener('waiting', () => {
    const titleEl = document.getElementById('player-title');
    if (titleEl && state.currentTrack) {
      titleEl.textContent = state.currentTrack.title + ' — loading...';
    }
  });

  audio.addEventListener('canplay', () => {
    const titleEl = document.getElementById('player-title');
    if (titleEl && state.currentTrack) {
      titleEl.textContent = state.currentTrack.title || 'Unknown';
    }
  });

  // ---- Global Click Delegation ----
  document.addEventListener('click', (e) => {
    const target = e.target.closest('[data-action]');
    if (!target) return;

    const action = target.dataset.action;

    switch (action) {
      case 'go-album':
        e.preventDefault();
        navigateTo('album-detail', { id: target.dataset.id });
        break;

      case 'go-artist':
        e.preventDefault();
        navigateTo('artist-detail', { id: target.dataset.id });
        break;

      case 'go-artists':
        e.preventDefault();
        navigateTo('artists');
        break;

      case 'go-albums':
        e.preventDefault();
        navigateTo('albums');
        break;

      case 'go-network':
        e.preventDefault();
        navigateTo('network');
        break;

      case 'play-album': {
        e.preventDefault();
        const albumId = target.dataset.id;
        api('/albums/' + albumId).then(album => {
          if (!album || !album.tracks) return;
          const q = album.tracks.map(t => ({ track: t, albumId: album.id, isRemote: false }));
          playQueue(q, 0);
        }).catch(() => {});
        break;
      }

      case 'shuffle-album': {
        e.preventDefault();
        const albumId = target.dataset.id;
        api('/albums/' + albumId).then(album => {
          if (!album || !album.tracks) return;
          const shuffled = [...album.tracks].sort(() => Math.random() - 0.5);
          const q = shuffled.map(t => ({ track: t, albumId: album.id, isRemote: false }));
          playQueue(q, 0);
        }).catch(() => {});
        break;
      }

      case 'play-album-track': {
        e.preventDefault();
        const albumId = target.dataset.albumId;
        const index = parseInt(target.dataset.index);
        api('/albums/' + albumId).then(album => {
          if (!album || !album.tracks) return;
          const q = album.tracks.map(t => ({ track: t, albumId: album.id, isRemote: false }));
          playQueue(q, index);
        }).catch(() => {});
        break;
      }

      case 'play-search-track': {
        e.preventDefault();
        const trackId = target.dataset.trackId;
        const searchEl = document.getElementById('search-results');
        const allTracks = searchEl?._searchTracks || [];
        const idx = parseInt(target.dataset.index);
        if (allTracks.length) {
          const q = allTracks.map(t => ({ track: t, albumId: t.albumId, isRemote: false }));
          playQueue(q, idx);
        } else {
          api('/tracks/' + trackId).then(track => {
            if (!track) return;
            playQueue([{ track, albumId: track.albumId, isRemote: false }], 0);
          }).catch(() => {});
        }
        break;
      }

      case 'view-peer': {
        e.preventDefault();
        if (e.target.closest('[data-action="sync-peer"]') || e.target.closest('[data-action="unfollow-peer"]')) break;
        const peerId = target.dataset.peerId;
        const peerName = target.dataset.peerName;
        showPeerLibrary(peerId, peerName);
        break;
      }

      case 'view-remote-album': {
        e.preventDefault();
        const peerId = target.dataset.peerId;
        const albumId = target.dataset.albumId;
        const peerName = target.dataset.peerName;
        showRemoteAlbum(peerId, albumId, peerName);
        break;
      }

      case 'sync-peer': {
        e.preventDefault();
        e.stopPropagation();
        const peerId = target.dataset.peerId;
        api('/peers/' + peerId + '/sync', { method: 'POST' })
          .then(() => toast('Sync triggered'))
          .catch(err => toast('Sync error: ' + err.message));
        break;
      }

      case 'unfollow-peer': {
        e.preventDefault();
        e.stopPropagation();
        const peerId = target.dataset.peerId;
        if (confirm('Unfollow this peer?')) {
          api('/peers/' + peerId, { method: 'DELETE' })
            .then(() => { toast('Peer unfollowed'); renderNetwork(); })
            .catch(err => toast('Error: ' + err.message));
        }
        break;
      }

      case 'close-modal':
        e.preventDefault();
        closeModal();
        break;
    }
  });

  // Close modal on overlay click
  $modal.addEventListener('click', (e) => {
    if (e.target === $modal) closeModal();
  });

  // ---- Keyboard Shortcuts ----
  document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') {
      if (e.key === 'Escape') e.target.blur();
      return;
    }

    if (e.key === 'Escape' && !$modal.classList.contains('hidden')) {
      closeModal();
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
        document.getElementById('volume-slider').value = state.volume;
        break;
      case 'ArrowDown':
        e.preventDefault();
        state.volume = Math.max(0, state.volume - 0.05);
        audio.volume = state.volume;
        document.getElementById('volume-slider').value = state.volume;
        break;
    }
  });

  // ---- Init ----
  checkAuth();
})();
