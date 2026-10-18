# Domains and HTTPS

Moonshed has a reverse proxy built in: an embedded Caddy server. You never edit Caddy's config yourself. Moonshed generates it from your domains and settings, and Caddy takes care of HTTP/2, HTTP/3, WebSockets and certificates.

## Turn on the proxy

The proxy is off until you enable it in **Settings › Proxy**. While it's off, apps are only reachable directly on their port.

When it's on, it listens on `http_addr` and `https_addr` from the [configuration file](/docs/reference/configuration), `:80` and `:443` by default. Set a **Let's Encrypt email** there if you want expiry notices from Let's Encrypt.

## Add a domain to an app

On the app's **Domains** tab, add a host such as `app.example.com`. Point its DNS record at your server first.

A domain can also have a path, so `example.com/api` and `example.com` can go to different apps. The default path is `/`.

The proxy forwards requests to `127.0.0.1:<app port>`. An app without a port gets no routes.

### HTTPS

Each domain picks how it does TLS:

- **Let's Encrypt:** certificates are issued and renewed automatically. Port 80 must be reachable from the internet.
- **Custom certificate:** upload a certificate and key as PEM, or a PFX file. Moonshed checks that it covers the host and hasn't expired. Paths on the same host share the certificate, and it's removed once no domain uses it.
- **None:** plain HTTP.

Uploaded certificates are listed under **Settings › Certificates**.

### Domain options

Open a domain to change:

- **Redirect HTTP to HTTPS** (on by default) and **HSTS**
- **Redirect www.** to the bare host
- **Compress responses**
- **Strip the path** before passing requests on, for apps that don't know they live under a prefix
- **Max request body**
- **Basic auth** with one or more users
- **Allowed IP addresses**
- **Request and response headers** to set or remove

For anything else, the domain's **Advanced** tab takes raw Caddyfile directives for that domain.

## Global proxy settings

**Settings › Proxy** also has:

- **Trusted proxies**, for when Moonshed sits behind a load balancer or Cloudflare, so client IPs are read from the forwarded headers.
- **Global options** and **Extra sites** as raw Caddyfile, for things Moonshed doesn't cover.
- A view of the **generated Caddyfile**.

## Safety

Every change is validated before it's saved. If the config only fails because of raw Caddyfile someone typed, the proxy runs without those parts and shows the error, so one bad snippet can't take every app offline.

An unchanged config isn't reloaded, since a reload drops idle keep-alive connections.

## The panel

The panel is always served on `panel_addr` (`:5050` by default) over plain HTTP, even when the proxy is off or misconfigured, so you can't lock yourself out.

To reach the panel over HTTPS, set a **Panel domain** in **Settings › General** with Let's Encrypt, a custom certificate or plain HTTP. Both ways keep working.

Once the panel has a domain, consider setting `panel_addr: "127.0.0.1:5050"` so port 5050 isn't exposed to the network.
