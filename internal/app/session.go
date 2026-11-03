package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"time"
)

// Settings sessions.
//
// The settings page never contains the control key. Any program or other
// account on this computer can fetch http://127.0.0.1:<port>/, so a key in the
// page would hand them full control. Instead, Smart Search itself opens the
// page with a one-time code. The page swaps that code for a cookie that is
// HttpOnly, SameSite=Lax and only valid with this data folder's key, then
// calls the API with the cookie plus a custom header. Lax rather than Strict
// so Stremio's Configure button, which reaches /configure through a link on
// stremio.com, still finds the session; cross-site requests that could change
// anything are already refused for lacking the custom header. A browser that never
// received a code sees a short page saying how to open settings.

const (
	// settingsCodeTTL is how long a one-time code waits for the browser.
	settingsCodeTTL = 2 * time.Minute
	// maxSettingsCodes bounds codes that were never used.
	maxSettingsCodes = 16
	// sessionMaxAge is the longest lifetime current browsers accept, so a
	// bookmark keeps working in a browser Smart Search has opened before.
	sessionMaxAge = 400 * 24 * 60 * 60
	// sessionHeader must accompany cookie-authenticated API calls. A custom
	// header cannot be sent cross-site without a CORS preflight, which /api/
	// never answers.
	sessionHeader = "X-Smart-Search-Session"
)

// sessionValue is derived from the control key, so sessions survive restarts
// and the cookie never reveals the key itself.
func (a *App) sessionValue() string {
	mac := hmac.New(sha256.New, []byte(a.Token))
	mac.Write([]byte("stremio-smart-search settings session v1"))
	return hex.EncodeToString(mac.Sum(nil))
}

// sessionCookieName includes the port, because browsers share cookies
// between ports and two copies on different ports have different keys.
func (a *App) sessionCookieName() string {
	name := "sss_session"
	if base, err := url.Parse(a.BaseURL); err == nil && base.Port() != "" {
		name += "_" + base.Port()
	}
	return name
}

func (a *App) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(a.sessionCookieName())
	return err == nil && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(a.sessionValue())) == 1
}

// SettingsURL returns a link to the settings page carrying a fresh one-time code.
func (a *App) SettingsURL() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return a.BaseURL + "/"
	}
	code := hex.EncodeToString(bytes)
	now := time.Now()
	a.codesMu.Lock()
	if a.codes == nil {
		a.codes = map[string]time.Time{}
	}
	for c, expires := range a.codes {
		if now.After(expires) {
			delete(a.codes, c)
		}
	}
	for c := range a.codes {
		if len(a.codes) < maxSettingsCodes {
			break
		}
		delete(a.codes, c)
	}
	a.codes[code] = now.Add(settingsCodeTTL)
	a.codesMu.Unlock()
	return a.BaseURL + "/?code=" + code
}

// redeemCode uses up a one-time code and reports whether it was valid.
func (a *App) redeemCode(code string) bool {
	if code == "" {
		return false
	}
	a.codesMu.Lock()
	defer a.codesMu.Unlock()
	expires, ok := a.codes[code]
	delete(a.codes, code)
	return ok && time.Now().Before(expires)
}

// OpenSettings opens the settings page in the default browser.
func (a *App) OpenSettings() error { return OpenURL(a.SettingsURL()) }

func (a *App) setSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: a.sessionCookieName(), Value: a.sessionValue(), Path: "/", MaxAge: sessionMaxAge, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func randomNonce() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// lockedPage is shown to a browser that Smart Search has not opened. It has
// no key, no script and no data.
const lockedPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Stremio Smart Search</title>
<style nonce="NONCE">body{font:16px/1.5 system-ui,sans-serif;max-width:34rem;margin:15vh auto;padding:0 1rem;color:#1d1d1f;background:#fff}h1{font-size:1.3rem}@media (prefers-color-scheme:dark){body{color:#f2f2f2;background:#121212}}</style>
</head><body><main>
<h1>Open Smart Search Settings from your computer</h1>
<p>For your security, settings open only from Smart Search itself. Use the <b>Smart Search Settings</b> shortcut in the Start menu or app menu, or the Smart Search icon in the notification area.</p>
<p>After that, this browser remembers it, so bookmarks and Stremio's Configure button work here too.</p>
</main></body></html>
`
