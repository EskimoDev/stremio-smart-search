(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  let timer, stopped = false, clients = [], lastStatus, selectedProfile, selectedLanguage, selectedRegion, regionOptionsFor;
  const clientNames = { stremio: 'Stremio', dxbi: 'dx.bi', enhanced: 'Stremio Enhanced' };
  const installButtons = [...document.querySelectorAll('[data-client]')];
  const base = window.smartSearch.baseURL;
  $('manifest').value = `${base}/manifest.json`;
  const tell = message => { $('message').textContent = message; };
  async function api(path, method = 'GET', data) {
    const options = { method, headers: { 'X-Smart-Search-Session': '1' }, credentials: 'same-origin' };
    if (data) { options.headers['Content-Type'] = 'application/json'; options.body = JSON.stringify(data); }
    const response = await fetch(`/api/${path}`, options);
    const result = await response.json();
    if (!response.ok) { const error = new Error(result.error || 'The helper could not complete that request.'); error.status = response.status; throw error; }
    return result;
  }
  const option = (value, text) => { const node = document.createElement('option'); node.value = value; node.textContent = text; return node; };
  const languageOf = (status, id) => (status.languages || []).find(language => language.id === id);
  function renderAliases(status) {
    const languageSelect = $('alias-language'), regionSelect = $('alias-region');
    if (languageSelect.options.length <= 1 && status.languages?.length) {
      for (const language of status.languages) languageSelect.append(option(language.id, language.name));
    }
    languageSelect.value = selectedLanguage;
    languageSelect.disabled = status.building;
    const language = languageOf(status, selectedLanguage);
    const regions = language?.regions || [];
    if (regionOptionsFor !== selectedLanguage) {
      regionOptionsFor = selectedLanguage;
      regionSelect.replaceChildren(option('', 'All regions'), ...regions.map(region => option(region.id, region.name)));
    }
    regionSelect.value = selectedRegion;
    regionSelect.hidden = $('alias-region-label').hidden = regions.length === 0;
    regionSelect.disabled = status.building;
    const changed = selectedLanguage !== (status.language || '') || selectedRegion !== (status.region || '');
    const region = regions.find(region => region.id === selectedRegion);
    let info = !language ? 'Also find movies and series by their title in another language or region.'
      : `Find titles by their ${language.name} names${region ? ` used in ${region.name}` : ''}, as well as their usual names.`;
    if (changed && status.ready && !status.building) info += ' Takes effect after rebuilding; the current catalogue keeps working meanwhile.';
    else if (language && status.ready && !changed) info += ` ${status.aliases.toLocaleString()} alternative titles in this catalogue.`;
    if (language && (changed || !status.ready)) info += ' Adds IMDb’s alternative-title file (about 500 MB) to the download.';
    $('alias-info').textContent = info;
    return changed;
  }
  // Screen readers hear the download's stage when it changes and its progress
  // every 15 seconds, not every one-second poll.
  let spokenStage = '', spokenAt = 0;
  function announceProgress(status, text) {
    if (!status.building) { spokenStage = ''; return; }
    const now = Date.now();
    if (status.stage === spokenStage && now - spokenAt < 15000) return;
    spokenStage = status.stage; spokenAt = now;
    $('progress-live').textContent = text;
  }
  const megabytes = bytes => `${Math.round(bytes / 1e6).toLocaleString()} MB`;
  const when = value => {
    const date = new Date(value), now = new Date();
    const day = date.toDateString() === now.toDateString() ? 'today' : date.toLocaleDateString(undefined, { weekday: 'short', day: 'numeric', month: 'short' });
    return `${day}, ${date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}`;
  };
  const frequencyNames = { 24: 'every day', 48: 'every 2 days', 72: 'every 3 days', 168: 'every week' };
  let updateSaving = false;
  function renderUpdates(status) {
    const updates = status.updates;
    if (!updates) return;
    if (!updateSaving) {
      $('auto-update').checked = updates.auto;
      $('update-frequency').value = String(updates.intervalHours);
    }
    $('auto-update').disabled = updateSaving;
    $('update-frequency').disabled = updateSaving || !updates.auto;
    const size = `about ${megabytes(updates.downloadBytes)}`;
    const weekly = updates.aliasBytes ? `, plus ${megabytes(updates.aliasBytes)} once a week for alternative titles` : '';
    const next = updates.nextCheck && new Date(updates.nextCheck) > new Date() ? ` Next check ${when(updates.nextCheck)}.` : ' Next check soon.';
    let info;
    if (!status.ready) info = updates.auto
      ? `After setup, Smart Search checks IMDb ${frequencyNames[updates.intervalHours]} and downloads ${size} only when there is new data. Turn this off to update only when you choose.`
      : 'Automatic updates are off. Use Refresh now after setup to get new titles.';
    else if (!updates.auto) info = 'Automatic updates are off. Choose Refresh now to get new titles.';
    else if (status.building) info = updates.automatic ? 'Updating automatically. Search keeps working with your current catalogue.' : 'Refreshing now. Search keeps working with your current catalogue.';
    else if (updates.state === 'checking') info = 'Checking IMDb for new data…';
    else if (updates.state === 'metered') info = 'Paused on a metered connection. Refresh now still works.';
    else if (updates.state === 'retrying') info = `The last update didn’t finish (${updates.lastError || 'unknown error'}). Your current catalogue is unchanged.${next}`;
    else if (updates.checkResult === 'unchanged') info = `Up to date: IMDb had no new data at ${when(updates.lastCheck)}.${next}`;
    else info = `Downloads ${size} only when IMDb has new data${weekly}.${next}`;
    $('update-info').textContent = info;
  }
  async function saveUpdates(change) {
    updateSaving = true; if (lastStatus) renderUpdates(lastStatus);
    try { const status = await api('updates', 'POST', change); updateSaving = false; render(status); }
    catch (error) { updateSaving = false; tell(error.message); if (lastStatus) renderUpdates(lastStatus); }
  }
  function render(status) {
    if (!selectedProfile || (lastStatus?.building && !status.building && !status.error)) {
      selectedProfile = status.buildProfile || status.profile;
      selectedLanguage = status.building ? status.buildLanguage || '' : status.language || '';
      selectedRegion = status.building ? status.buildRegion || '' : status.region || '';
    }
    lastStatus = status;
    $('catalog-profile').value = selectedProfile;
    $('catalog-profile').disabled = status.building;
    const profile = status.profiles.find(profile => profile.id === selectedProfile);
    $('profile-info').textContent = profile ? `Up to ${profile.movies.toLocaleString()} movies + ${profile.series.toLocaleString()} series. Minimum ${profile.minVotes} IMDb votes. Actual totals depend on the source.` : 'Choose a catalogue size.';
    $('connection').textContent = status.building ? 'Preparing catalogue' : status.standby ? 'Standby' : status.ready ? 'Running locally' : 'Catalogue needed';
    $('connection').classList.toggle('ready', status.ready);
    $('connection').classList.toggle('building', status.building);
    $('build').disabled = status.building;
    const sizeChanged = selectedProfile !== status.profile, titlesChanged = renderAliases(status);
    $('build').textContent = status.building ? 'Preparing…' : !status.ready ? 'Prepare catalogue'
      : sizeChanged && titlesChanged ? 'Apply changes & rebuild' : sizeChanged ? 'Apply size & rebuild' : titlesChanged ? 'Apply titles & rebuild' : 'Refresh now';
    $('progress').hidden = !status.building;
    const stage = `${status.stage}${status.bytes ? ` · ${(status.bytes / 1024 / 1024).toFixed(1)} MB downloaded` : ''}`;
    $('stage').textContent = stage;
    announceProgress(status, stage);
    // First run: the download is step 01 and the Stremio steps follow it.
    $('prepare-step').hidden = status.ready;
    $('prepare').disabled = status.building;
    $('prepare').textContent = status.building ? 'Preparing…' : 'Prepare catalogue';
    $('prepare-info').textContent = status.building ? stage : `Uses the ${profile?.name || 'Standard'} size. You can change it under Title catalogue.`;
    const first = status.ready ? 1 : 2;
    ['step-choose', 'step-paste', 'step-order'].forEach((id, index) => { $(id).textContent = `0${first + index}`; });
    $('catalog-stats').hidden = !status.ready;
    $('movie-count').textContent = status.movies.toLocaleString();
    $('series-count').textContent = status.series.toLocaleString();
    const activeProfile = status.profiles.find(profile => profile.id === status.profile);
    const activeLanguage = languageOf(status, status.language);
    const updated = status.updates?.lastSuccess || status.builtAt;
    $('catalog-info').textContent = status.ready ? `${activeProfile?.name || 'Local'}${activeLanguage ? ` · ${activeLanguage.name} titles` : ''} · updated ${when(updated)}` : 'Download IMDb title data to enable setup. A few hundred MB; allow several minutes.';
    renderUpdates(status);
    renderRunMode(status);
    for (const button of installButtons) {
      const client = clients.find(client => client.id === button.dataset.client);
      // aria-disabled keeps the buttons focusable, so keyboard and screen-reader
      // users still reach the reason in their status line.
      button.setAttribute('aria-disabled', String(!status.ready || !client?.available));
      $(`client-status-${button.dataset.client}`).textContent = !client ? 'Detection unavailable · use the link below' : !client.available ? 'Not detected · use the link below' : !status.ready ? 'Detected · prepare catalogue first' : 'Detected on this computer';
    }
    $('web-install').classList.toggle('disabled', !status.ready);
    $('web-install').setAttribute('aria-disabled', String(!status.ready));
    if (status.ready) $('web-install').href = status.webInstall;
    else $('web-install').removeAttribute('href');
    $('launch').disabled = !status.ready;
    $('launch').textContent = `Open ${clientNames[status.preferredClient] || 'default Stremio app'}`;
    $('version').textContent = `${status.product} · ${status.version}`;
    if (status.error) tell(`${status.error}${status.ready ? ' Your previous catalogue is still available.' : ' You can try preparing the catalogue again.'}`);
  }
  const platform = navigator.userAgentData?.platform || navigator.platform || '';
  const folderLabel = /^win/i.test(platform) ? 'Show in Explorer' : /mac/i.test(platform) ? 'Show in Finder' : 'Open folder';
  $('open-folder').textContent = folderLabel;
  const size = bytes => bytes >= 1024 ** 4 ? `${(bytes / 1024 ** 4).toFixed(2)} TB` : bytes >= 1024 ** 3 ? `${(bytes / 1024 ** 3).toFixed(bytes >= 100 * 1024 ** 3 ? 0 : 1)} GB` : bytes >= 1024 ** 2 ? `${(bytes / 1024 ** 2).toFixed(1)} MB` : bytes >= 1024 ? `${Math.round(bytes / 1024)} KB` : `${bytes} B`;
  async function refreshStorage() {
    try {
      const storage = await api('storage');
      const segments = { catalogue: storage.catalogue, backup: storage.backup, other: storage.other };
      for (const [name, bytes] of Object.entries(segments)) {
        $(`seg-${name}`).style.width = storage.total ? `${(bytes / storage.total * 100).toFixed(2)}%` : '0%';
        $(`seg-${name}`).style.minWidth = bytes ? '3px' : '0';
        $(`seg-${name}`).title = `${name === 'backup' ? 'Previous catalogue' : name === 'other' ? 'Settings' : 'Catalogue'}: ${size(bytes)}`;
      }
      $('storage-total').textContent = size(storage.total);
      $('storage-bar').setAttribute('aria-label', `Smart Search uses ${size(storage.total)}: catalogue ${size(storage.catalogue)}, previous catalogue ${size(storage.backup)}, settings ${size(storage.other)}`);
      $('storage-catalogue').textContent = storage.catalogue ? size(storage.catalogue) : 'None yet';
      $('storage-backup-row').hidden = !storage.backup;
      $('storage-backup').textContent = size(storage.backup);
      $('storage-other').textContent = size(storage.other);
      $('storage-disk').textContent = storage.diskSize ? `${size(storage.diskFree)} of ${size(storage.diskSize)}` : 'Unknown';
      $('storage-memory').textContent = size(storage.memory);
      $('storage-path').textContent = storage.path;
      $('storage-path').title = storage.custom ? 'Custom location' : 'Default location';
      $('change-folder').disabled = storage.moving;
      $('reset-folder').hidden = $('reset-sep').hidden = !storage.custom;
      lastStorage = storage;
      $('open-folder').disabled = !storage.canOpen;
    } catch { $('storage-path').textContent = 'Storage details unavailable'; }
  }
  let storageFor, lastStorage, resetMode = false;
  function openMovePanel(path, info, reset = false) {
    resetMode = reset;
    $('move-panel').hidden = false;
    $('move-path').value = path;
    $('move-path').disabled = reset;
    $('move-confirm').disabled = false;
    $('move-confirm').textContent = reset ? 'Move back' : 'Move catalogue';
    $('move-info').textContent = info;
    if (!reset && !path) $('move-path').focus();
  }
  const moveSummary = () => {
    const bytes = (lastStorage?.catalogue || 0) + (lastStorage?.backup || 0);
    return `${bytes ? `Moves ${size(bytes)} of catalogue files` : 'Future catalogues are saved'} into a “Stremio Smart Search” folder there. Settings stay where they are, and search keeps working while it moves.`;
  };
  async function refresh() {
    if (stopped) return;
    try {
      const status = await api('status'); render(status); timer = setTimeout(refresh, status.building ? 1000 : 10000);
      // Sizes only change when a catalogue is saved, so skip the walk on ordinary polls.
      const key = status.building ? null : `${status.ready}|${status.builtAt}`;
      if (key && key !== storageFor) { storageFor = key; refreshStorage(); }
    }
    catch { $('connection').textContent = 'Helper is not running'; tell('Open Smart Search again to reconnect.'); }
  }
  async function build() {
    clearTimeout(timer); tell('');
    try { render(await api('build', 'POST', { profile: selectedProfile, language: selectedLanguage, region: selectedRegion })); } catch (error) { tell(error.message); }
    refresh();
  }
  $('build').addEventListener('click', build);
  $('prepare').addEventListener('click', build);
  $('auto-update').addEventListener('change', () => saveUpdates({ auto: $('auto-update').checked }));
  $('update-frequency').addEventListener('change', () => saveUpdates({ intervalHours: Number($('update-frequency').value) }));
  $('catalog-profile').addEventListener('change', () => {
    selectedProfile = $('catalog-profile').value;
    if (lastStatus) render(lastStatus);
  });
  $('alias-language').addEventListener('change', () => {
    selectedLanguage = $('alias-language').value;
    selectedRegion = '';
    if (lastStatus) render(lastStatus);
  });
  $('alias-region').addEventListener('change', () => {
    selectedRegion = $('alias-region').value;
    if (lastStatus) render(lastStatus);
  });
  $('copy').addEventListener('click', async () => {
    try { await navigator.clipboard.writeText($('manifest').value); tell('Addon link copied. Paste it into your app’s Add addon dialog.'); }
    catch { $('manifest').focus(); $('manifest').select(); tell('Select and copy the addon link above.'); }
  });
  for (const button of installButtons) button.addEventListener('click', async () => {
    const id = button.dataset.client;
    if (!lastStatus?.ready) { tell('Prepare the catalogue first, then choose your app.'); return; }
    if (!clients.find(client => client.id === id)?.available) { tell(`${clientNames[id]} wasn’t detected. Copy the addon link below and paste it into the app yourself.`); return; }
    try { await navigator.clipboard.writeText($('manifest').value); }
    catch { $('manifest').focus(); $('manifest').select(); tell('Copy the local link below, then paste it into your app’s Add addon dialog.'); return; }
    try {
      const result = await api('install', 'POST', { client: id });
      render({ ...lastStatus, preferredClient: id });
      const instruction = `Link copied. In ${result.name}, open Addons → Add addon, paste the link and choose Install.`;
      $('install-help').textContent = instruction; tell(instruction);
    } catch (error) { $('install-help').textContent = error.message; tell(error.message); }
  });
  $('launch').addEventListener('click', async () => {
    try { await api('launch', 'POST'); tell(`Opening ${clientNames[lastStatus?.preferredClient] || 'your default Stremio app'}.`); } catch (error) { tell(error.message); }
  });
  $('open-folder').addEventListener('click', async () => {
    try { await api('storage/open', 'POST'); $('storage-info').textContent = 'Opened the data folder.'; }
    catch (error) { $('storage-info').textContent = error.message; }
  });
  $('change-folder').addEventListener('click', async () => {
    $('storage-info').textContent = '';
    if (lastStorage && !lastStorage.canChoose) { openMovePanel('', moveSummary()); return; }
    const button = $('change-folder');
    button.disabled = true; button.textContent = 'Choose a folder in the window that opened…';
    try {
      const result = await api('storage/choose', 'POST');
      if (!result.cancelled) openMovePanel(result.path, moveSummary());
    } catch (error) { openMovePanel('', error.message); }
    finally { button.disabled = false; button.textContent = 'Change location…'; }
  });
  $('reset-folder').addEventListener('click', () => {
    $('storage-info').textContent = '';
    openMovePanel(lastStorage?.defaultPath || '', 'Moves the catalogue back to the default folder next to your settings.', true);
  });
  $('move-cancel').addEventListener('click', () => { $('move-panel').hidden = true; });
  $('move-path').addEventListener('keydown', event => {
    if (event.key === 'Enter') $('move-confirm').click();
    if (event.key === 'Escape') $('move-cancel').click();
  });
  $('move-confirm').addEventListener('click', async () => {
    const path = resetMode ? '' : $('move-path').value.trim();
    if (!resetMode && !path) { $('move-info').textContent = 'Enter or choose a folder first.'; return; }
    $('move-confirm').disabled = true; $('move-confirm').textContent = 'Moving…';
    try {
      const result = await api('storage/move', 'POST', { path });
      $('move-panel').hidden = true;
      $('storage-info').textContent = `Catalogue moved to ${result.path}`;
      refreshStorage();
    } catch (error) {
      $('move-info').textContent = error.message;
      $('move-confirm').disabled = false; $('move-confirm').textContent = resetMode ? 'Move back' : 'Move catalogue';
    }
  });
  // Quitting stops search in Stremio until the helper is started again, so a
  // stray click only arms the button.
  // It stays armed until focus leaves it, with no timer to race.
  let quitArmed = false;
  $('quit').addEventListener('click', async () => {
    if (!quitArmed) {
      quitArmed = true;
      $('quit').textContent = 'Click again to quit';
      tell('Click Quit helper again to stop Smart Search. Its rows disappear from Stremio until you open it again.');
      return;
    }
    stopped = true; clearTimeout(timer);
    try { await api('quit', 'POST'); $('connection').textContent = 'Helper stopped'; tell('Smart Search has stopped. You can close this page.'); }
    catch (error) { tell(error.message); }
  });
  $('quit').addEventListener('blur', () => {
    if (!quitArmed || stopped) return;
    quitArmed = false; $('quit').textContent = 'Quit helper'; tell('');
  });
  // Run mode: always loaded, or a light standby while no Stremio app is open.
  let runModeSaving = false, signInStart = true;
  function renderRunMode(status) {
    $('run-mode-settings').hidden = !status.ready || !status.runMode;
    if (runModeSaving) return;
    const withStremio = status.runMode === 'stremio';
    $('run-with-stremio').checked = withStremio;
    $('run-with-stremio').disabled = false;
    let info = !withStremio ? 'Off: search stays loaded and answers instantly. On: Smart Search waits in a light standby of about 20 MB and loads search when Stremio, dx.bi or Stremio Enhanced opens.'
      : status.standby ? 'In standby. Search loads when Stremio, dx.bi or Stremio Enhanced opens. A search from a browser loads it straight away and takes a moment longer.'
      : 'Search is loaded. It goes back into standby a few minutes after Stremio closes.';
    if (withStremio && !signInStart) info += ' Also turn on starting at sign-in, so Smart Search is waiting when you open Stremio.';
    $('run-mode-info').textContent = info;
  }
  $('run-with-stremio').addEventListener('change', async () => {
    runModeSaving = true; $('run-with-stremio').disabled = true;
    try { const status = await api('run-mode', 'POST', { mode: $('run-with-stremio').checked ? 'stremio' : 'always' }); runModeSaving = false; render(status); }
    catch (error) { runModeSaving = false; $('run-with-stremio').checked = !$('run-with-stremio').checked; $('run-with-stremio').disabled = false; $('run-mode-info').textContent = error.message; }
  });
  const autostartInfo = 'Smart Search rows only appear in Stremio while the helper is running. Turn this on if you open Stremio with its own shortcut.';
  function renderAutostart(result) {
    $('autostart-settings').hidden = !result.supported;
    $('autostart').checked = result.enabled;
    signInStart = !result.supported || result.enabled;
    if (lastStatus) renderRunMode(lastStatus);
    $('autostart').disabled = false;
    $('autostart-info').textContent = result.enabled ? `Starts quietly when you sign in, using ${result.path}. Settings stay closed.` : autostartInfo;
  }
  api('autostart').then(renderAutostart).catch(() => {});
  $('autostart').addEventListener('change', async () => {
    $('autostart').disabled = true;
    try { renderAutostart(await api('autostart', 'POST', { enabled: $('autostart').checked })); }
    catch (error) { $('autostart').checked = !$('autostart').checked; $('autostart').disabled = false; $('autostart-info').textContent = error.message; }
  });
  api('clients').then(result => {
    clients = result.clients;
    const missing = clients.filter(client => !client.available).map(client => client.name);
    $('install-help').textContent = missing.length ? 'If your app is portable or not detected, use the addon link below.' : '';
    if (lastStatus) render(lastStatus);
  }).catch(() => { $('install-help').textContent = 'Use the web option or copy the link if your app is not detected.'; });
  refresh();
})();
