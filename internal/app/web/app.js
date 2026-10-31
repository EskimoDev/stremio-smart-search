(() => {
  'use strict';
  const $ = id => document.getElementById(id);
  let timer, stopped = false;
  const base = window.smartSearch.baseURL;
  $('manifest').value = `${base}/manifest.json`;
  const tell = message => { $('message').textContent = message; };
  async function api(path, method = 'GET') {
    const response = await fetch(`/api/${path}`, { method, headers: { 'X-Smart-Search-Key': window.smartSearch.token } });
    const result = await response.json();
    if (!response.ok) throw new Error(result.error || 'The helper could not complete that request.');
    return result;
  }
  function render(status) {
    $('connection').textContent = status.building ? 'Preparing catalogue' : status.ready ? 'Ready · running locally' : 'Setup needed';
    $('connection').classList.toggle('ready', status.ready);
    $('build').disabled = status.building;
    $('build').textContent = status.building ? 'Preparing…' : status.ready ? 'Refresh catalogue' : 'Prepare catalogue';
    $('progress').hidden = !status.building;
    $('stage').textContent = `${status.stage}${status.bytes ? ` · ${(status.bytes / 1024 / 1024).toFixed(1)} MB downloaded` : ''}`;
    $('catalog-info').textContent = status.ready ? `${status.movies.toLocaleString()} movies · ${status.series.toLocaleString()} series · updated ${new Date(status.builtAt).toLocaleDateString()}` : '';
    $('install').classList.toggle('disabled', !status.ready);
    $('install').setAttribute('aria-disabled', String(!status.ready));
    if (status.ready) $('install').href = status.manifest.replace(/^http:\/\//, 'stremio://');
    else $('install').removeAttribute('href');
    $('launch').disabled = !status.ready;
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
  $('launch').addEventListener('click', async () => {
    try { await api('launch', 'POST'); tell('Opening your default Stremio app.'); } catch (error) { tell(error.message); }
  });
  $('quit').addEventListener('click', async () => {
    stopped = true; clearTimeout(timer);
    try { await api('quit', 'POST'); $('connection').textContent = 'Helper stopped'; tell('Smart Search has stopped. You can close this page.'); }
    catch (error) { tell(error.message); }
  });
  refresh();
})();
