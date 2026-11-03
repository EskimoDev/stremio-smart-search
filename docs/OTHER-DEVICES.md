# TVs, phones and other devices

Smart Search is a desktop helper today. This page covers what works on other devices and what is still untested.

## What works today

Android, Android TV, Fire TV, iOS and tvOS do not currently have native companion packages. A localhost addon installed on your PC does not become a running helper on those devices through account sync.

## Home server over HTTPS

Advanced users can host the companion on a home server and expose its addon endpoints through HTTPS. This avoids maintaining a separate engine on each device, but the host must remain online and client support must be checked. Stremio clients can require HTTPS for addresses other than loopback. Plain HTTP to a LAN address is not a promised cross-platform installation route.

## LAN mode (untested on TVs)

Running the companion with `--listen 0.0.0.0:11479` on a PC lets other devices on your network reach `http://<pc-ip>:11479/manifest.json`. Only the addon manifest, search and logo are exposed; settings and controls stay on the PC. Whether Stremio on Fire TV or Android TV accepts that plain-HTTP address has not been tested yet. See [platform testing](PLATFORM-TESTING.md#fire-tv-and-android-tv-lan-mode) for the steps to try it.

For HTTPS hosting, run the helper behind an HTTPS reverse proxy and expose only the addon routes: `/manifest.json`, `/catalog/*` and `/addon-logo.png`. Preserve the external host, and account for the fact that the helper derives its logo scheme from the incoming TLS connection, not forwarded scheme headers. TLS termination at a proxy needs separate logo-URL verification; it is not established by the direct-HTTPS tests. The settings interface accepts loopback clients only, by design.

Native Android/TV packages, iOS/tvOS companions, a hosted public service and automatic cloud deployment are outside the current preview.
