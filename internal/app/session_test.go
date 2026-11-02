package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func pageRequest(a *App, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11479"+target, nil)
	r.RemoteAddr = "127.0.0.1:32100"
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

// Any program or account on the computer can fetch the settings page, so it
// must never carry the control key or a working session by itself.
func TestSettingsPageNeverContainsTheKey(t *testing.T) {
	a := testApp(t)
	for _, path := range []string{"/", "/?code=guess", "/configure"} {
		w := pageRequest(a, path)
		if strings.Contains(w.Body.String(), a.Token) || strings.Contains(w.Body.String(), a.sessionValue()) || len(w.Result().Cookies()) != 0 {
			t.Fatalf("%s exposed the key or a session", path)
		}
	}
	if w := pageRequest(a, "/"); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Smart Search Settings") {
		t.Fatalf("browser without a session got %d: %s", w.Code, w.Body)
	}
}

func TestOneTimeCodeOpensASession(t *testing.T) {
	a := testApp(t)
	link, _ := url.Parse(a.SettingsURL())
	w := pageRequest(a, link.RequestURI())
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("code redirect %d %q", w.Code, w.Header().Get("Location"))
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Value == a.Token {
		t.Fatalf("unexpected session cookie %+v", cookies)
	}
	session := cookies[0]
	page := pageRequest(a, "/", session)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "window.smartSearch") || strings.Contains(page.Body.String(), a.Token) {
		t.Fatalf("settings page with session: %d", page.Code)
	}
	// The nonce is fresh per page, never the key.
	if csp := page.Header().Get("Content-Security-Policy"); strings.Contains(csp, a.Token) || !strings.Contains(csp, "'nonce-") {
		t.Fatalf("CSP %q", csp)
	}
	// The code works once.
	if again := pageRequest(a, link.RequestURI()); len(again.Result().Cookies()) != 0 {
		t.Fatal("one-time code was accepted twice")
	}

	api := func(session *http.Cookie, header, origin string) int {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11479/api/status", nil)
		r.RemoteAddr = "127.0.0.1:32100"
		if session != nil {
			r.AddCookie(session)
		}
		if header != "" {
			r.Header.Set(sessionHeader, header)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if code := api(session, "1", ""); code != 200 {
		t.Fatalf("session API call refused: %d", code)
	}
	if code := api(session, "", ""); code != 403 {
		t.Fatalf("session without the custom header accepted: %d", code)
	}
	if code := api(session, "1", "https://unrelated.example"); code != 403 {
		t.Fatalf("session from a foreign origin accepted: %d", code)
	}
	forged := &http.Cookie{Name: session.Name, Value: strings.Repeat("0", len(session.Value))}
	if code := api(forged, "1", ""); code != 403 {
		t.Fatalf("forged session accepted: %d", code)
	}
}

func TestExpiredCodeIsRefused(t *testing.T) {
	a := testApp(t)
	link, _ := url.Parse(a.SettingsURL())
	code := link.Query().Get("code")
	a.codesMu.Lock()
	a.codes[code] = a.codes[code].Add(-2 * settingsCodeTTL)
	a.codesMu.Unlock()
	if w := pageRequest(a, link.RequestURI()); len(w.Result().Cookies()) != 0 {
		t.Fatal("expired code accepted")
	}
	for i := 0; i < 3*maxSettingsCodes; i++ {
		a.SettingsURL()
	}
	if len(a.codes) > maxSettingsCodes {
		t.Fatalf("%d unused codes kept", len(a.codes))
	}
}

func TestSessionsAreTiedToTheKey(t *testing.T) {
	a, b := testApp(t), testApp(t)
	if a.sessionValue() == b.sessionValue() {
		t.Fatal("different keys gave the same session")
	}
	if a.sessionCookieName() != "sss_session_11479" {
		t.Fatalf("cookie name %q", a.sessionCookieName())
	}
}
