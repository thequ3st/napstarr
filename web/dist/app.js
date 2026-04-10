// ============================================
// Napstarr — Midnight Vinyl SPA
// ============================================

(function () {
  'use strict';

  // ---- State ----
  var state = {
    user: null,
    artists: [],
    albums: [],
    currentAlbum: null,
    currentArtist: null,
    queue: [],
    queueIndex: 0,
    playing: false,
    currentTrack: null,
    volume: 0.8,
    isRemote: false,
    remotePeerId: null,
    remotePeerName: null,
    stats: null,
    instance: null,
    peers: []
  };

  // ---- Audio ----
  var audio = new Audio();
  audio.volume = state.volume;
  audio.preload = 'metadata';

  // ---- DOM refs ----
  var $loginScreen = document.getElementById('login-screen');
  var $appShell = document.getElementById('app-shell');
  var $loginForm = document.getElementById('login-form');
  var $loginError = document.getElementById('login-error');
  var $viewContainer = document.getElementById('view-container');
  var $playerBar = document.getElementById('player-bar');
  var $playerTitle = document.getElementById('player-title');
  var $playerArtist = document.getElementById('player-artist');
  var $playerArt = document.getElementById('player-art');
  var $playerTime = document.getElementById('player-time');
  var $playerProgressFill = document.getElementById('player-progress-fill');
  var $playerProgressWrap = document.getElementById('player-progress-wrap');
  var $playerPlayBtn = document.getElementById('player-play-btn');
  var $playIcon = document.getElementById('play-icon');
  var $pauseIcon = document.getElementById('pause-icon');
  var $playerRemoteBadge = document.getElementById('player-remote-badge');
  var $playerRemoteName = document.getElementById('player-remote-name');
  var $volumeSlider = document.getElementById('volume-slider');
  var $modalOverlay = document.getElementById('modal-overlay');
  var $modalContent = document.getElementById('modal-content');
  var $toastContainer = document.getElementById('toast-container');
  var $instanceInfo = document.getElementById('instance-info');

  // ---- Helpers ----
  function esc(str) {
    if (!str) return '';
    var d = document.createElement('div');
    d.textContent = String(str);
    return d.innerHTML;
  }

  function fmtDuration(ms) {
    if (!ms && ms !== 0) return '0:00';
    var totalSec = Math.floor(ms / 1000);
    var m = Math.floor(totalSec / 60);
    var s = totalSec % 60;
    return m + ':' + (s < 10 ? '0' : '') + s;
  }

  function fmtDurationSec(sec) {
    if (!sec && sec !== 0) return '0:00';
    sec = Math.floor(sec);
    var m = Math.floor(sec / 60);
    var s = sec % 60;
    return m + ':' + (s < 10 ? '0' : '') + s;
  }

  function fmtDurationLong(ms) {
    if (!ms) return '0 min';
    var totalMin = Math.floor(ms / 60000);
    if (totalMin < 60) return totalMin + ' min';
    var hr = Math.floor(totalMin / 60);
    var min = totalMin % 60;
    return hr + ' hr' + (min > 0 ? ' ' + min + ' min' : '');
  }

  function fmtSize(bytes) {
    if (!bytes) return '0 B';
    var units = ['B', 'KB', 'MB', 'GB', 'TB'];
    var i = 0;
    var v = bytes;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return v.toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
  }

  function getInitials(name) {
    if (!name) return '?';
    var parts = name.trim().split(/\s+/);
    if (parts.length === 1) return parts[0].charAt(0).toUpperCase();
    return (parts[0].charAt(0) + parts[parts.length - 1].charAt(0)).toUpperCase();
  }

  function staggerClass(i) {
    var n = Math.min(i + 1, 12);
    return 'stagger-' + n;
  }

  // ---- Toast ----
  function toast(message, type) {
    type = type || 'info';
    var el = document.createElement('div');
    el.className = 'toast toast-' + type;
    el.textContent = message;
    $toastContainer.appendChild(el);
    setTimeout(function () {
      el.style.opacity = '0';
      setTimeout(function () { el.remove(); }, 300);
    }, 3500);
  }

  // ---- API Client ----
  async function api(path, options) {
    options = options || {};
    var headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    var fetchOpts = Object.assign({}, options, { credentials: 'same-origin', headers: headers });
    // Don't set Content-Type for GET/HEAD (no body)
    if (!options.body) delete fetchOpts.headers['Content-Type'];
    var res;
    try {
      res = await fetch('/api' + path, fetchOpts);
    } catch (e) {
      throw new Error('Network error');
    }
    if (res.status === 401) {
      showLogin();
      throw new Error('unauthorized');
    }
    if (!res.ok) {
      var d = {};
      try { d = await res.json(); } catch (_) {}
      throw new Error(d.error || 'HTTP ' + res.status);
    }
    if (res.status === 204) return null;
    return res.json();
  }

  // ---- Auth ----
  function showLogin() {
    $loginScreen.hidden = false;
    $appShell.hidden = true;
    state.user = null;
  }

  function showApp() {
    $loginScreen.hidden = true;
    $appShell.hidden = false;
  }

  $loginForm.addEventListener('submit', async function (e) {
    e.preventDefault();
    $loginError.hidden = true;
    var user = document.getElementById('login-user').value.trim();
    var pass = document.getElementById('login-pass').value;
    try {
      var data = await api('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username: user, password: pass })
      });
      state.user = data || { username: user };
      showApp();
      navigate(location.hash || '#home');
      loadInstanceInfo();
    } catch (err) {
      $loginError.textContent = err.message === 'unauthorized' ? 'Invalid credentials' : err.message;
      $loginError.hidden = false;
    }
  });

  // ---- Navigation ----
  function navigate(hash) {
    hash = hash || '#home';
    var view = hash.replace('#', '').split('/')[0];
    var param = hash.split('/')[1] || null;
    var param2 = hash.split('/')[2] || null;

    // Update nav active states
    document.querySelectorAll('.nav-link, .mobile-tab').forEach(function (el) {
      var v = el.getAttribute('data-view');
      el.classList.toggle('active', v === view || (v === 'home' && view === ''));
    });

    // Route
    switch (view) {
      case '':
      case 'home':
        renderHome();
        break;
      case 'artists':
        if (param) renderArtistDetail(param);
        else renderArtists();
        break;
      case 'albums':
        if (param) renderAlbumDetail(param);
        else renderAlbums();
        break;
      case 'search':
        renderSearch();
        break;
      case 'network':
        if (param === 'peer' && param2) renderPeerBrowse(param2);
        else renderNetwork();
        break;
      default:
        renderHome();
    }

    // Scroll to top
    document.querySelector('.main-content').scrollTop = 0;
  }

  window.addEventListener('hashchange', function () {
    navigate(location.hash);
  });

  // ---- Render Helpers ----
  function setView(html) {
    $viewContainer.innerHTML = '<div class="fade-in">' + html + '</div>';
  }

  function spinnerHTML() {
    return '<div class="spinner"></div>';
  }

  function emptyHTML(msg) {
    return '<div class="empty-state">' +
      '<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/></svg>' +
      '<p>' + esc(msg) + '</p></div>';
  }

  function albumCardHTML(album, index) {
    var art = album.artwork ? esc(album.artwork) : '';
    var artImg = art ? '<img src="' + art + '" alt="" loading="lazy">' : '<div style="width:100%;height:100%;background:var(--elevated);display:flex;align-items:center;justify-content:center;color:var(--text-muted)"><svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/></svg></div>';
    return '<div class="album-card slide-up ' + staggerClass(index) + '" data-action="go-album" data-id="' + esc(album.id) + '">' +
      '<div class="album-card-art">' + artImg +
      '<div class="album-play-overlay"><div class="album-play-btn" data-action="play-album" data-id="' + esc(album.id) + '"><svg viewBox="0 0 24 24"><polygon points="6,3 20,12 6,21"/></svg></div></div>' +
      '</div>' +
      '<div class="album-card-title">' + esc(album.title || album.name) + '</div>' +
      '<div class="album-card-artist">' + esc(album.artist_name || album.artist || '') + '</div>' +
      '</div>';
  }

  function artistCardHTML(artist, index) {
    var hasImg = artist.image || artist.artwork;
    var avatarContent = hasImg
      ? '<img src="' + esc(artist.image || artist.artwork) + '" alt="" loading="lazy">'
      : esc(getInitials(artist.name));
    return '<div class="artist-card slide-up ' + staggerClass(index) + '" data-action="go-artist" data-id="' + esc(artist.id) + '">' +
      '<div class="artist-avatar">' + avatarContent + '</div>' +
      '<div class="artist-card-name">' + esc(artist.name) + '</div>' +
      '<div class="artist-card-count">' + (artist.album_count || 0) + ' album' + ((artist.album_count || 0) === 1 ? '' : 's') + '</div>' +
      '</div>';
  }

  function trackRowHTML(track, index, albumId, isRemote, peerId, peerName) {
    var isCurrent = state.currentTrack && state.currentTrack.id === track.id;
    var cls = 'track-row' + (isCurrent ? ' playing' : '');
    var remote = isRemote ? ' data-remote="1" data-peer-id="' + esc(peerId) + '" data-peer-name="' + esc(peerName || '') + '"' : '';
    return '<div class="' + cls + '" data-action="play-track" data-track-id="' + esc(track.id) + '" data-album-id="' + esc(albumId || '') + '" data-index="' + index + '"' + remote + '>' +
      '<div class="track-num">' + (index + 1) + '</div>' +
      '<div class="track-title">' + esc(track.title || track.name) + '</div>' +
      '<div class="track-duration">' + fmtDuration(track.duration) + '</div>' +
      '</div>';
  }

  // ---- Views ----

  // Home
  async function renderHome() {
    setView(spinnerHTML());
    try {
      var results = await Promise.all([
        api('/stats').catch(function () { return {}; }),
        api('/albums?sort=recent&limit=20').catch(function () { return []; })
      ]);
      var stats = results[0] || {};
      var recentAlbums = results[1] || [];
      // Normalize: API may return {albums:[]} or just []
      if (recentAlbums.albums) recentAlbums = recentAlbums.albums;

      state.stats = stats;

      var html = '<div class="view-header"><h1>Home</h1></div>';

      // Stats
      html += '<div class="stats-row">';
      html += '<div class="stat-card"><div class="stat-value">' + (stats.artist_count || stats.artists || 0) + '</div><div class="stat-label">Artists</div></div>';
      html += '<div class="stat-card"><div class="stat-value">' + (stats.album_count || stats.albums || 0) + '</div><div class="stat-label">Albums</div></div>';
      html += '<div class="stat-card"><div class="stat-value">' + (stats.track_count || stats.tracks || 0) + '</div><div class="stat-label">Tracks</div></div>';
      html += '<div class="stat-card"><div class="stat-value">' + fmtSize(stats.library_size || stats.size || 0) + '</div><div class="stat-label">Library Size</div></div>';
      html += '</div>';

      // Actions
      html += '<div class="action-row">';
      html += '<button class="btn-outline" data-action="scan-library"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 12a9 9 0 11-6.22-8.56"/><polyline points="21 3 21 9 15 9"/></svg>Scan Library</button>';
      html += '<button class="btn-outline" data-action="enrich-metadata"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>Enrich Metadata</button>';
      html += '</div>';

      // Recent albums
      if (recentAlbums.length > 0) {
        html += '<h2 class="section-title">Recently Added</h2>';
        html += '<div class="album-grid">';
        recentAlbums.forEach(function (a, i) {
          html += albumCardHTML(a, i);
        });
        html += '</div>';
      } else {
        html += emptyHTML('No albums yet. Scan your library to get started.');
      }

      setView(html);
    } catch (err) {
      if (err.message !== 'unauthorized') {
        setView(emptyHTML('Failed to load: ' + err.message));
      }
    }
  }

  // Artists
  async function renderArtists() {
    setView(spinnerHTML());
    try {
      var data = await api('/artists');
      var artists = data || [];
      if (data && data.artists) artists = data.artists;
      state.artists = artists;

      var html = '<div class="view-header"><h1>Artists</h1><p>' + artists.length + ' artist' + (artists.length === 1 ? '' : 's') + '</p></div>';

      if (artists.length === 0) {
        html += emptyHTML('No artists found.');
      } else {
        html += '<div class="artist-grid">';
        artists.forEach(function (a, i) {
          html += artistCardHTML(a, i);
        });
        html += '</div>';
      }

      setView(html);
    } catch (err) {
      if (err.message !== 'unauthorized') setView(emptyHTML('Failed to load artists.'));
    }
  }

  // Artist Detail
  async function renderArtistDetail(id) {
    setView(spinnerHTML());
    try {
      var results = await Promise.all([
        api('/artists/' + encodeURIComponent(id)),
        api('/artists/' + encodeURIComponent(id) + '/albums').catch(function () { return []; })
      ]);
      var artist = results[0] || {};
      var albums = results[1] || [];
      if (albums.albums) albums = albums.albums;
      state.currentArtist = artist;

      var hasImg = artist.image || artist.artwork;
      var avatarContent = hasImg
        ? '<img src="' + esc(artist.image || artist.artwork) + '" alt="">'
        : esc(getInitials(artist.name));

      var html = '<div class="artist-detail-header slide-up">';
      html += '<div class="artist-detail-avatar">' + avatarContent + '</div>';
      html += '<div class="artist-detail-info"><h1>' + esc(artist.name) + '</h1>';
      html += '<div class="meta">' + albums.length + ' album' + (albums.length === 1 ? '' : 's') + '</div>';
      html += '</div></div>';

      if (albums.length > 0) {
        html += '<h2 class="section-title">Albums</h2>';
        html += '<div class="album-grid">';
        albums.forEach(function (a, i) {
          html += albumCardHTML(a, i);
        });
        html += '</div>';
      } else {
        html += emptyHTML('No albums for this artist.');
      }

      setView(html);
    } catch (err) {
      if (err.message !== 'unauthorized') setView(emptyHTML('Failed to load artist.'));
    }
  }

  // Albums
  async function renderAlbums() {
    setView(spinnerHTML());
    try {
      var data = await api('/albums');
      var albums = data || [];
      if (data && data.albums) albums = data.albums;
      state.albums = albums;

      var html = '<div class="view-header"><h1>Albums</h1><p>' + albums.length + ' album' + (albums.length === 1 ? '' : 's') + '</p></div>';

      if (albums.length === 0) {
        html += emptyHTML('No albums found. Scan your library to discover music.');
      } else {
        html += '<div class="album-grid">';
        albums.forEach(function (a, i) {
          html += albumCardHTML(a, i);
        });
        html += '</div>';
      }

      setView(html);
    } catch (err) {
      if (err.message !== 'unauthorized') setView(emptyHTML('Failed to load albums.'));
    }
  }

  // Album Detail
  async function renderAlbumDetail(id) {
    setView(spinnerHTML());
    try {
      var album = await api('/albums/' + encodeURIComponent(id));
      album = album || {};
      state.currentAlbum = album;
      var tracks = album.tracks || [];
      var totalDuration = tracks.reduce(function (sum, t) { return sum + (t.duration || 0); }, 0);

      var art = album.artwork ? '<img src="' + esc(album.artwork) + '" alt="">' : '<div style="width:100%;height:100%;background:var(--elevated);display:flex;align-items:center;justify-content:center;color:var(--text-muted)"><svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/></svg></div>';

      var html = '<div class="album-detail-header slide-up">';
      html += '<div class="album-detail-art">' + art + '</div>';
      html += '<div class="album-detail-info">';
      html += '<h1>' + esc(album.title || album.name) + '</h1>';
      html += '<div class="album-detail-artist" data-action="go-artist" data-id="' + esc(album.artist_id || '') + '">' + esc(album.artist_name || album.artist || 'Unknown Artist') + '</div>';
      html += '<div class="album-detail-meta">';
      if (album.year) html += '<span>' + esc(album.year) + '</span>';
      html += '<span>' + tracks.length + ' track' + (tracks.length === 1 ? '' : 's') + '</span>';
      html += '<span>' + fmtDurationLong(totalDuration) + '</span>';
      if (album.genre) html += '<span>' + esc(album.genre) + '</span>';
      html += '</div>';
      html += '<div class="album-play-all"><button class="btn-primary" data-action="play-album" data-id="' + esc(album.id) + '">Play All</button></div>';
      html += '</div></div>';

      // Track list
      if (tracks.length > 0) {
        html += '<div class="track-list">';
        html += '<div class="track-list-header"><div>#</div><div>Title</div><div style="text-align:right">Duration</div></div>';
        tracks.forEach(function (t, i) {
          html += trackRowHTML(t, i, album.id, false, null, null);
        });
        html += '</div>';
      }

      setView(html);
    } catch (err) {
      if (err.message !== 'unauthorized') setView(emptyHTML('Failed to load album.'));
    }
  }

  // Search
  var searchDebounce = null;

  function renderSearch() {
    var html = '<div class="view-header"><h1>Search</h1></div>';
    html += '<div class="search-input-wrap">';
    html += '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>';
    html += '<input type="text" class="search-input" id="search-input" placeholder="Search tracks, albums, artists..." autofocus>';
    html += '</div>';
    html += '<div id="search-results"></div>';
    setView(html);

    var input = document.getElementById('search-input');
    if (input) {
      input.addEventListener('input', function () {
        clearTimeout(searchDebounce);
        var q = input.value.trim();
        if (!q) {
          document.getElementById('search-results').innerHTML = '';
          return;
        }
        searchDebounce = setTimeout(function () {
          doSearch(q);
        }, 300);
      });
      input.focus();
    }
  }

  async function doSearch(query) {
    var container = document.getElementById('search-results');
    if (!container) return;
    container.innerHTML = spinnerHTML();
    try {
      var data = await api('/search?q=' + encodeURIComponent(query));
      var tracks = data || [];
      if (data && data.tracks) tracks = data.tracks;
      if (data && data.results) tracks = data.results;

      if (tracks.length === 0) {
        container.innerHTML = emptyHTML('No results for "' + esc(query) + '"');
        return;
      }

      var html = '<div class="track-list">';
      html += '<div class="track-list-header"><div>#</div><div>Title</div><div style="text-align:right">Duration</div></div>';
      tracks.forEach(function (t, i) {
        html += trackRowHTML(t, i, t.album_id || '', false, null, null);
      });
      html += '</div>';
      container.innerHTML = html;
    } catch (err) {
      container.innerHTML = emptyHTML('Search failed: ' + err.message);
    }
  }

  // Network
  async function renderNetwork() {
    setView(spinnerHTML());
    try {
      var results = await Promise.all([
        api('/instance').catch(function () { return {}; }),
        api('/peers').catch(function () { return []; })
      ]);
      var instance = results[0] || {};
      var peers = results[1] || [];
      if (peers.peers) peers = peers.peers;
      state.instance = instance;
      state.peers = peers;

      var html = '<div class="view-header"><h1>Network</h1></div>';

      // Instance info card
      html += '<div class="instance-card"><h3>Your Instance</h3>';
      html += '<dl class="meta-grid">';
      html += '<dt>Name</dt><dd>' + esc(instance.name || 'Unnamed') + '</dd>';
      html += '<dt>Instance ID</dt><dd>' + esc(instance.id || instance.instance_id || 'N/A') + '</dd>';
      html += '<dt>Peer ID</dt><dd>' + esc(instance.peer_id || 'N/A') + '</dd>';
      if (instance.addresses && instance.addresses.length) {
        html += '<dt>Addresses</dt><dd>' + instance.addresses.map(esc).join('<br>') + '</dd>';
      }
      html += '</dl></div>';

      // Follow new peer
      html += '<h2 class="section-title">Peers</h2>';
      html += '<div class="peer-follow-row">';
      html += '<input type="text" class="peer-follow-input" id="peer-address-input" placeholder="Enter peer address to follow...">';
      html += '<button class="btn-primary" data-action="follow-peer">Follow</button>';
      html += '</div>';

      // Peer list
      if (peers.length === 0) {
        html += emptyHTML('No peers followed yet.');
      } else {
        html += '<div class="peer-list">';
        peers.forEach(function (p) {
          var isActive = p.status === 'active' || p.status === 'online' || p.connected;
          html += '<div class="peer-card">';
          html += '<div class="peer-status ' + (isActive ? 'active' : 'inactive') + '"></div>';
          html += '<div class="peer-info">';
          html += '<div class="peer-name">' + esc(p.name || p.instance_name || 'Unknown Peer') + '</div>';
          html += '<div class="peer-meta">' + esc(p.instance_id || p.id || '') + (p.last_synced ? ' &middot; Synced ' + esc(p.last_synced) : '') + '</div>';
          html += '</div>';
          html += '<div class="peer-actions">';
          html += '<button class="btn-sm btn-sm-accent" data-action="browse-peer" data-id="' + esc(p.id) + '" data-name="' + esc(p.name || p.instance_name || 'Peer') + '">Browse</button>';
          html += '<button class="btn-sm btn-sm-accent" data-action="sync-peer" data-id="' + esc(p.id) + '">Sync</button>';
          html += '<button class="btn-sm btn-sm-ghost" data-action="unfollow-peer" data-id="' + esc(p.id) + '">Unfollow</button>';
          html += '</div>';
          html += '</div>';
        });
        html += '</div>';
      }

      setView(html);
    } catch (err) {
      if (err.message !== 'unauthorized') setView(emptyHTML('Failed to load network.'));
    }
  }

  // Peer Browse Modal
  async function openPeerBrowseModal(peerId, peerName) {
    $modalContent.innerHTML = spinnerHTML();
    $modalOverlay.hidden = false;
    try {
      var results = await Promise.all([
        api('/peers/' + encodeURIComponent(peerId) + '/artists').catch(function () { return []; }),
        api('/peers/' + encodeURIComponent(peerId) + '/albums').catch(function () { return []; })
      ]);
      var artists = results[0] || [];
      var albums = results[1] || [];
      if (artists.artists) artists = artists.artists;
      if (albums.albums) albums = albums.albums;

      var html = '<button class="modal-close" data-action="close-modal">&times;</button>';
      html += '<h2 style="font-family:var(--font-heading);font-weight:700;margin-bottom:4px">' + esc(peerName) + '</h2>';
      html += '<p style="color:var(--text-secondary);font-size:0.85rem;margin-bottom:24px">' + artists.length + ' artists &middot; ' + albums.length + ' albums</p>';

      if (albums.length > 0) {
        html += '<h3 class="section-title" style="font-size:1rem">Albums</h3>';
        html += '<div class="album-grid" style="grid-template-columns:repeat(auto-fill,minmax(130px,1fr));gap:16px">';
        albums.forEach(function (a, i) {
          var art = a.artwork ? '<img src="' + esc(a.artwork) + '" alt="" loading="lazy">' : '<div style="width:100%;height:100%;background:var(--elevated);display:flex;align-items:center;justify-content:center;color:var(--text-muted)"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="3"/></svg></div>';
          html += '<div class="album-card slide-up ' + staggerClass(i) + '" data-action="browse-peer-album" data-album-id="' + esc(a.id) + '" data-peer-id="' + esc(peerId) + '" data-peer-name="' + esc(peerName) + '">';
          html += '<div class="album-card-art" style="aspect-ratio:1;border-radius:6px;overflow:hidden;background:var(--elevated)">' + art + '</div>';
          html += '<div class="album-card-title" style="margin-top:8px;font-size:0.82rem">' + esc(a.title || a.name) + '</div>';
          html += '<div class="album-card-artist" style="font-size:0.75rem">' + esc(a.artist_name || a.artist || '') + '</div>';
          html += '</div>';
        });
        html += '</div>';
      }

      if (artists.length > 0 && albums.length === 0) {
        html += '<h3 class="section-title" style="font-size:1rem">Artists</h3>';
        html += '<div class="artist-grid" style="grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:12px">';
        artists.forEach(function (a, i) {
          html += '<div class="artist-card slide-up ' + staggerClass(i) + '" style="cursor:default">';
          html += '<div class="artist-avatar" style="width:64px;height:64px;font-size:1rem">' + esc(getInitials(a.name)) + '</div>';
          html += '<div class="artist-card-name" style="font-size:0.8rem">' + esc(a.name) + '</div>';
          html += '</div>';
        });
        html += '</div>';
      }

      if (albums.length === 0 && artists.length === 0) {
        html += emptyHTML('This peer\'s library is empty.');
      }

      $modalContent.innerHTML = html;
    } catch (err) {
      $modalContent.innerHTML = '<button class="modal-close" data-action="close-modal">&times;</button>' + emptyHTML('Failed to browse peer: ' + err.message);
    }
  }

  // Browse peer album (show tracks in modal)
  async function openPeerAlbumModal(albumId, peerId, peerName) {
    $modalContent.innerHTML = spinnerHTML();
    $modalOverlay.hidden = false;
    try {
      var album = await api('/peers/' + encodeURIComponent(peerId) + '/albums/' + encodeURIComponent(albumId));
      album = album || {};
      var tracks = album.tracks || [];

      var html = '<button class="modal-close" data-action="close-modal">&times;</button>';
      html += '<h2 style="font-family:var(--font-heading);font-weight:700;margin-bottom:2px">' + esc(album.title || album.name) + '</h2>';
      html += '<p style="color:var(--accent);font-size:0.9rem;margin-bottom:4px">' + esc(album.artist_name || album.artist || '') + '</p>';
      html += '<p style="color:var(--text-muted);font-size:0.8rem;margin-bottom:20px">From ' + esc(peerName) + ' &middot; ' + tracks.length + ' tracks</p>';

      if (tracks.length > 0) {
        html += '<div class="track-list">';
        html += '<div class="track-list-header"><div>#</div><div>Title</div><div style="text-align:right">Duration</div></div>';
        tracks.forEach(function (t, i) {
          html += trackRowHTML(t, i, albumId, true, peerId, peerName);
        });
        html += '</div>';
      }

      $modalContent.innerHTML = html;
    } catch (err) {
      $modalContent.innerHTML = '<button class="modal-close" data-action="close-modal">&times;</button>' + emptyHTML('Failed to load album.');
    }
  }

  function closeModal() {
    $modalOverlay.hidden = true;
    $modalContent.innerHTML = '';
  }

  // ---- Instance info in sidebar ----
  async function loadInstanceInfo() {
    try {
      var inst = await api('/instance');
      state.instance = inst || {};
      $instanceInfo.innerHTML = '<strong>' + esc(inst.name || 'Napstarr') + '</strong><br>' +
        '<span style="font-size:0.7rem;word-break:break-all">' + esc(inst.id || inst.instance_id || '') + '</span>';
    } catch (_) {
      $instanceInfo.innerHTML = '';
    }
  }

  // ---- Player ----
  function showPlayerBar() {
    $playerBar.hidden = false;
  }

  function playTrack(track, albumId, isRemote, peerId, peerName) {
    if (!track) return;

    state.currentTrack = track;
    state.isRemote = !!isRemote;
    state.remotePeerId = peerId || null;
    state.remotePeerName = peerName || null;

    // Build stream URL
    var src;
    if (isRemote && peerId) {
      src = '/api/stream/remote/' + encodeURIComponent(peerId) + '/' + encodeURIComponent(track.id);
    } else {
      src = '/api/stream/' + encodeURIComponent(track.id);
    }

    audio.src = src;
    audio.play().catch(function () {});

    // Update UI
    $playerTitle.textContent = track.title || track.name || 'Unknown';
    $playerArtist.textContent = track.artist_name || track.artist || '';
    if (track.artwork || track.album_artwork) {
      $playerArt.src = track.artwork || track.album_artwork;
      $playerArt.style.display = '';
    } else {
      $playerArt.src = '';
      $playerArt.style.display = 'none';
    }

    // Remote badge
    if (isRemote && peerName) {
      $playerRemoteBadge.hidden = false;
      $playerRemoteName.textContent = peerName;
    } else {
      $playerRemoteBadge.hidden = true;
    }

    showPlayerBar();
    updatePlayButton(true);

    // Highlight current track in list
    highlightCurrentTrack();
  }

  function playQueue(items, startIndex) {
    state.queue = items || [];
    state.queueIndex = startIndex || 0;
    if (state.queue.length === 0) return;
    var item = state.queue[state.queueIndex];
    playTrack(item.track, item.albumId, item.isRemote, item.peerId, item.peerName);
  }

  function togglePlay() {
    if (!state.currentTrack) return;
    if (audio.paused) {
      audio.play().catch(function () {});
    } else {
      audio.pause();
    }
  }

  function nextTrack() {
    if (state.queue.length === 0) return;
    state.queueIndex++;
    if (state.queueIndex >= state.queue.length) {
      state.queueIndex = 0;
    }
    var item = state.queue[state.queueIndex];
    playTrack(item.track, item.albumId, item.isRemote, item.peerId, item.peerName);
  }

  function prevTrack() {
    if (state.queue.length === 0) return;
    // If more than 3 seconds in, restart current track
    if (audio.currentTime > 3) {
      audio.currentTime = 0;
      return;
    }
    state.queueIndex--;
    if (state.queueIndex < 0) {
      state.queueIndex = state.queue.length - 1;
    }
    var item = state.queue[state.queueIndex];
    playTrack(item.track, item.albumId, item.isRemote, item.peerId, item.peerName);
  }

  function updatePlayButton(isPlaying) {
    state.playing = isPlaying;
    if (isPlaying) {
      $playIcon.hidden = true;
      $pauseIcon.hidden = false;
      $playerBar.classList.add('playing');
    } else {
      $playIcon.hidden = false;
      $pauseIcon.hidden = true;
      $playerBar.classList.remove('playing');
    }
  }

  function highlightCurrentTrack() {
    document.querySelectorAll('.track-row').forEach(function (row) {
      var tid = row.getAttribute('data-track-id');
      row.classList.toggle('playing', state.currentTrack && tid === String(state.currentTrack.id));
    });
  }

  // Audio events
  audio.addEventListener('timeupdate', function () {
    if (!audio.duration) return;
    var pct = (audio.currentTime / audio.duration) * 100;
    $playerProgressFill.style.width = pct + '%';
    $playerTime.textContent = fmtDurationSec(audio.currentTime) + ' / ' + fmtDurationSec(audio.duration);
  });

  audio.addEventListener('play', function () { updatePlayButton(true); });
  audio.addEventListener('pause', function () { updatePlayButton(false); });

  audio.addEventListener('ended', function () {
    updatePlayButton(false);
    // Record listen history for local tracks played >5s
    if (state.currentTrack && !state.isRemote && audio.currentTime > 5) {
      api('/history', {
        method: 'POST',
        body: JSON.stringify({ track_id: state.currentTrack.id })
      }).catch(function () {});
    }
    nextTrack();
  });

  audio.addEventListener('error', function () {
    var savedTitle = $playerTitle.textContent;
    $playerTitle.textContent = 'Unavailable';
    setTimeout(function () {
      $playerTitle.textContent = savedTitle;
      nextTrack();
    }, 800);
  });

  audio.addEventListener('waiting', function () {
    var original = $playerTitle.textContent;
    $playerTitle.setAttribute('data-original', original);
    $playerTitle.textContent = 'Loading...';
  });

  audio.addEventListener('canplay', function () {
    var original = $playerTitle.getAttribute('data-original');
    if (original) {
      $playerTitle.textContent = original;
      $playerTitle.removeAttribute('data-original');
    }
  });

  // Progress bar seek
  $playerProgressWrap.addEventListener('click', function (e) {
    if (!audio.duration) return;
    var rect = $playerProgressWrap.getBoundingClientRect();
    var pct = (e.clientX - rect.left) / rect.width;
    audio.currentTime = pct * audio.duration;
  });

  // Volume slider
  $volumeSlider.addEventListener('input', function () {
    var v = parseFloat($volumeSlider.value);
    audio.volume = v;
    state.volume = v;
  });

  // Keyboard shortcuts
  document.addEventListener('keydown', function (e) {
    var tag = (e.target.tagName || '').toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return;

    switch (e.code) {
      case 'Space':
        e.preventDefault();
        togglePlay();
        break;
      case 'ArrowLeft':
        e.preventDefault();
        if (audio.duration) audio.currentTime = Math.max(0, audio.currentTime - 10);
        break;
      case 'ArrowRight':
        e.preventDefault();
        if (audio.duration) audio.currentTime = Math.min(audio.duration, audio.currentTime + 10);
        break;
      case 'ArrowUp':
        e.preventDefault();
        state.volume = Math.min(1, state.volume + 0.05);
        audio.volume = state.volume;
        $volumeSlider.value = state.volume;
        break;
      case 'ArrowDown':
        e.preventDefault();
        state.volume = Math.max(0, state.volume - 0.05);
        audio.volume = state.volume;
        $volumeSlider.value = state.volume;
        break;
    }
  });

  // ---- Build queue from album tracks ----
  function buildAlbumQueue(tracks, albumId, startIndex, isRemote, peerId, peerName) {
    return tracks.map(function (t) {
      return {
        track: t,
        albumId: albumId,
        isRemote: !!isRemote,
        peerId: peerId || null,
        peerName: peerName || null
      };
    });
  }

  // ---- Play album by ID ----
  async function playAlbumById(albumId) {
    try {
      var album = await api('/albums/' + encodeURIComponent(albumId));
      album = album || {};
      var tracks = album.tracks || [];
      if (tracks.length === 0) {
        toast('No tracks in this album', 'error');
        return;
      }
      // Attach artwork to tracks
      tracks.forEach(function (t) {
        if (!t.artwork && !t.album_artwork) {
          t.album_artwork = album.artwork || '';
          t.artist_name = t.artist_name || album.artist_name || album.artist || '';
        }
      });
      var queue = buildAlbumQueue(tracks, albumId, 0, false, null, null);
      playQueue(queue, 0);
    } catch (err) {
      toast('Failed to play album', 'error');
    }
  }

  // ---- Event Delegation ----
  document.addEventListener('click', function (e) {
    var target = e.target.closest('[data-action]');
    if (!target) return;
    var action = target.getAttribute('data-action');

    switch (action) {
      case 'go-album': {
        var id = target.getAttribute('data-id');
        if (id) location.hash = '#albums/' + id;
        break;
      }
      case 'go-artist': {
        var aid = target.getAttribute('data-id');
        if (aid) location.hash = '#artists/' + aid;
        break;
      }
      case 'play-album': {
        e.stopPropagation();
        var albumId = target.getAttribute('data-id');
        if (albumId) playAlbumById(albumId);
        break;
      }
      case 'play-track': {
        var row = target.closest('.track-row');
        if (!row) break;
        var trackId = row.getAttribute('data-track-id');
        var aId = row.getAttribute('data-album-id');
        var idx = parseInt(row.getAttribute('data-index'), 10);
        var remote = row.getAttribute('data-remote') === '1';
        var pId = row.getAttribute('data-peer-id') || null;
        var pName = row.getAttribute('data-peer-name') || null;

        // Get all track rows in the same list
        var list = row.parentElement;
        var rows = list.querySelectorAll('.track-row');
        var queue = [];
        rows.forEach(function (r) {
          var t = {
            id: r.getAttribute('data-track-id'),
            title: r.querySelector('.track-title').textContent,
            duration: 0, // duration not critical for playback
            album_id: r.getAttribute('data-album-id')
          };
          // Try to get artwork from current album
          if (state.currentAlbum && state.currentAlbum.tracks) {
            var found = state.currentAlbum.tracks.find(function (ct) { return String(ct.id) === String(t.id); });
            if (found) {
              t = Object.assign({}, found);
              if (!t.artwork && !t.album_artwork) t.album_artwork = state.currentAlbum.artwork || '';
              if (!t.artist_name) t.artist_name = state.currentAlbum.artist_name || state.currentAlbum.artist || '';
            }
          }
          var isR = r.getAttribute('data-remote') === '1';
          queue.push({
            track: t,
            albumId: r.getAttribute('data-album-id'),
            isRemote: isR,
            peerId: r.getAttribute('data-peer-id') || null,
            peerName: r.getAttribute('data-peer-name') || null
          });
        });

        playQueue(queue, idx);
        break;
      }
      case 'play-single': {
        var tid = target.getAttribute('data-track-id');
        var track = { id: tid, title: target.getAttribute('data-title') || 'Track' };
        playTrack(track, null, false, null, null);
        break;
      }
      case 'toggle-play': {
        togglePlay();
        break;
      }
      case 'prev-track': {
        prevTrack();
        break;
      }
      case 'next-track': {
        nextTrack();
        break;
      }
      case 'scan-library': {
        toast('Scanning library...', 'info');
        api('/scan', { method: 'POST' }).then(function () {
          toast('Library scan started', 'success');
        }).catch(function (err) {
          toast('Scan failed: ' + err.message, 'error');
        });
        break;
      }
      case 'enrich-metadata': {
        toast('Enriching metadata...', 'info');
        api('/enrich', { method: 'POST' }).then(function () {
          toast('Metadata enrichment started', 'success');
        }).catch(function (err) {
          toast('Enrichment failed: ' + err.message, 'error');
        });
        break;
      }
      case 'follow-peer': {
        var input = document.getElementById('peer-address-input');
        if (!input) break;
        var addr = input.value.trim();
        if (!addr) { toast('Enter a peer address', 'error'); break; }
        api('/peers', { method: 'POST', body: JSON.stringify({ address: addr }) }).then(function () {
          toast('Peer followed', 'success');
          input.value = '';
          renderNetwork();
        }).catch(function (err) {
          toast('Failed to follow peer: ' + err.message, 'error');
        });
        break;
      }
      case 'unfollow-peer': {
        var pid = target.getAttribute('data-id');
        if (!pid) break;
        api('/peers/' + encodeURIComponent(pid), { method: 'DELETE' }).then(function () {
          toast('Peer unfollowed', 'success');
          renderNetwork();
        }).catch(function (err) {
          toast('Failed to unfollow: ' + err.message, 'error');
        });
        break;
      }
      case 'sync-peer': {
        var spid = target.getAttribute('data-id');
        if (!spid) break;
        toast('Syncing peer...', 'info');
        api('/peers/' + encodeURIComponent(spid) + '/sync', { method: 'POST' }).then(function () {
          toast('Sync started', 'success');
        }).catch(function (err) {
          toast('Sync failed: ' + err.message, 'error');
        });
        break;
      }
      case 'browse-peer': {
        var bpid = target.getAttribute('data-id');
        var bpname = target.getAttribute('data-name') || 'Peer';
        if (bpid) openPeerBrowseModal(bpid, bpname);
        break;
      }
      case 'browse-peer-album': {
        var bAlbumId = target.closest('[data-album-id]').getAttribute('data-album-id');
        var bPeerId = target.closest('[data-peer-id]').getAttribute('data-peer-id');
        var bPeerName = target.closest('[data-peer-name]').getAttribute('data-peer-name') || 'Peer';
        if (bAlbumId && bPeerId) openPeerAlbumModal(bAlbumId, bPeerId, bPeerName);
        break;
      }
      case 'close-modal': {
        closeModal();
        break;
      }
    }
  });

  // Close modal on Escape
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && !$modalOverlay.hidden) {
      closeModal();
    }
  });

  // ---- Init ----
  async function init() {
    // Try to check auth status
    try {
      var me = await api('/auth/me');
      state.user = me || {};
      showApp();
      navigate(location.hash || '#home');
      loadInstanceInfo();
    } catch (_) {
      showLogin();
    }
  }

  init();

})();
