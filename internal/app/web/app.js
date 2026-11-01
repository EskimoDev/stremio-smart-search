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
    const options = { method, headers: { 'X-Smart-Search-Key': window.smartSearch.token } };
    if (data) { options.headers['Content-Type'] = 'application/json'; options.body = JSON.stringify(data); }
    const response = await fetch(`/api/${path}`, options);
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || 'The helper could not complete that request.');
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
    // Measured: Expanded already exceeds the memory target, so alternative titles slow it; Standard is unaffected.
    if (language && selectedProfile === 'expanded') info += ' Standard is recommended with alternative titles: Expanded uses the most memory and searches more slowly with them.';
    $('alias-info').textContent = info;
    return changed;
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
    $('connection').textContent = status.building ? 'Preparing catalogue' : status.ready ? 'Running locally' : 'Catalogue needed';
    $('connection').classList.toggle('ready', status.ready);
    $('connection').classList.toggle('building', status.building);
    $('build').disabled = status.building;
    const sizeChanged = selectedProfile !== status.profile, titlesChanged = renderAliases(status);
    $('build').textContent = status.building ? 'Preparing…' : !status.ready ? 'Prepare catalogue'
      : sizeChanged && titlesChanged ? 'Apply changes & rebuild' : sizeChanged ? 'Apply size & rebuild' : titlesChanged ? 'Apply titles & rebuild' : 'Refresh catalogue';
    $('progress').hidden = !status.building;
    $('stage').textContent = `${status.stage}${status.bytes ? ` · ${(status.bytes / 1024 / 1024).toFixed(1)} MB downloaded` : ''}`;
    $('catalog-stats').hidden = !status.ready;
    $('movie-count').textContent = status.movies.toLocaleString();
    $('series-count').textContent = status.series.toLocaleString();
    const activeProfile = status.profiles.find(profile => profile.id === status.profile);
    const activeLanguage = languageOf(status, status.language);
    $('catalog-info').textContent = status.ready ? `${activeProfile?.name || 'Local'}${activeLanguage ? ` · ${activeLanguage.name} titles` : ''} · updated ${new Date(status.builtAt).toLocaleDateString()}` : 'Download IMDb title data to enable setup. A few hundred MB; allow several minutes.';
    for (const button of installButtons) {
      const client = clients.find(client => client.id === button.dataset.client);
      button.disabled = !status.ready || !client?.available;
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
  async function refresh() {
    if (stopped) return;
    try { const status = await api('status'); render(status); timer = setTimeout(refresh, status.building ? 1000 : 10000); }
    catch { $('connection').textContent = 'Helper is not running'; tell('Open Smart Search again to reconnect.'); }
  }
  $('build').addEventListener('click', async () => {
    clearTimeout(timer); tell('');
    try { render(await api('build', 'POST', { profile: selectedProfile, language: selectedLanguage, region: selectedRegion })); } catch (error) { tell(error.message); }
    refresh();
  });
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
  $('quit').addEventListener('click', async () => {
    stopped = true; clearTimeout(timer);
    try { await api('quit', 'POST'); $('connection').textContent = 'Helper stopped'; tell('Smart Search has stopped. You can close this page.'); }
    catch (error) { tell(error.message); }
  });
  api('clients').then(result => {
    clients = result.clients;
    const missing = clients.filter(client => !client.available).map(client => client.name);
    $('install-help').textContent = missing.length ? 'If your app is portable or not detected, use the addon link below.' : '';
    if (lastStatus) render(lastStatus);
  }).catch(() => { $('install-help').textContent = 'Use the web option or copy the link if your app is not detected.'; });
  refresh();
})();
