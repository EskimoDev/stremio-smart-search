(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  let timer, stopped = false, clients = [], lastStatus;
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
  function render(status) {
    lastStatus = status;
    $('connection').textContent = status.building ? 'Preparing catalogue' : status.ready ? 'Ready · running locally' : 'Setup needed';
    $('connection').classList.toggle('ready', status.ready);
    $('build').disabled = status.building;
    $('build').textContent = status.building ? 'Preparing…' : status.ready ? 'Refresh catalogue' : 'Prepare catalogue';
    $('progress').hidden = !status.building;
    $('stage').textContent = `${status.stage}${status.bytes ? ` · ${(status.bytes / 1024 / 1024).toFixed(1)} MB downloaded` : ''}`;
    $('catalog-info').textContent = status.ready ? `${status.movies.toLocaleString()} movies · ${status.series.toLocaleString()} series · updated ${new Date(status.builtAt).toLocaleDateString()}` : '';
    for (const button of installButtons) {
      const client = clients.find(client => client.id === button.dataset.client);
      button.disabled = !status.ready || !client?.available;
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
    try { render(await api('build', 'POST')); } catch (error) { tell(error.message); }
    refresh();
  });
  $('copy').addEventListener('click', async () => {
    try { await navigator.clipboard.writeText($('manifest').value); tell('Addon link copied. Paste it into Stremio’s addon search.'); }
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
    $('install-help').textContent = missing.length ? `Not detected: ${missing.join(', ')}. Use the web option or copy the link if your app is portable.` : 'Stremio, dx.bi and Stremio Enhanced are available. Choose where to install.';
    if (lastStatus) render(lastStatus);
  }).catch(() => { $('install-help').textContent = 'Use the web option or copy the link if your app is not detected.'; });
  refresh();
})();
