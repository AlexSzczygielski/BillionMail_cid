# BillionMail — with inline CID image embedding

> **A personal fork of [BillionMail](https://github.com/aaPanel/BillionMail).  
> I'm not maintaining this. I just needed one thing and I built it.**

---

## Why this exists

BillionMail is a great self-hosted email marketing platform, but it sends images as external `<img src="https://...">` links. Some corporate mail gateways block images from unknown external domains by default — so your logo never renders for a chunk of recipients.

This fork adds **automatic inline CID image embedding**: any image hosted on your own BillionMail instance gets attached directly inside the MIME message instead of being hotlinked. No config toggle, no UI change — it just works at send time.

If this is the one thing you were missing too, feel free to use it.

---

## What's changed

One feature addition on top of the official BillionMail codebase:

- Self-hosted `<img>` tags (URLs pointing at your own instance) are rewritten to `src="cid:…"` at send time
- The image bytes are attached inline as `multipart/related` MIME parts
- External image URLs (third-party CDNs, etc.) are left completely untouched
- Plain-text emails and emails with no images behave exactly as before
- Image files are read from disk once per process and cached — no re-reads per recipient in bulk campaigns

---

## Install

**Prerequisites:** Docker + Docker Compose on your server.

```bash
git clone https://github.com/AlexSzczygielski/BillionMail_cid.git
cd BillionMail_cid
cp .env.example .env   # edit as required
```

### Build the core image

This fork ships a build script that compiles the Go binary inside a throwaway Alpine container — no Go installation required on the host:

```bash
# x86 (default)
./build-core.sh 4.9.3 x86

# ARM
./build-core.sh 4.9.3 arm
```

This produces a local Docker image tagged `billionmail-core-cid:4.9.3`. Your `docker-compose.yml` should reference that image name for the core service.

### Start everything

```bash
docker compose up -d
```

First boot takes a minute or two while it initialises the database and generates certificates.
> Follow the official BillionMail docs for DNS setup (MX, SPF, DKIM, DMARC). That part hasn't changed at all.

---

## Using CID image embedding

### How it works

When you send a campaign or transactional email, the pipeline checks every `<img src="…">` tag in the HTML. If the URL's hostname matches your BillionMail instance's own hostname, the image is embedded directly into the email as an inline attachment. Recipients' mail clients render it from the attachment — no external request, no blocked image.

### Step 1 — Put your image on the server

The image file needs to be physically present on the server inside `public/dist/` (the directory GoFrame uses as its static file root). Drop it in via SCP, a Docker volume, or your deploy process:

```bash
# 1. SCP the file to your server
scp logo.png user@your-server:/tmp/logo.png

# 2. Copy it into the running container
docker cp /tmp/logo.png <your-compose-project>-core-billionmail-1:/opt/billionmail/core/public/dist/logo.png
```

Not sure of your container name? Run `docker ps | grep core` to find it.

Or mount a directory in `docker-compose.yml` so files persist across rebuilds:

```yaml
volumes:
  - ./core-images:/opt/billionmail/core/public/dist/uploads
```

### Step 2 — Reference it in the email editor

In the BillionMail email editor, add an image block and paste the full URL pointing at your instance:

```
https://mail.yourdomain.com/logo.png
# or if you used a subdirectory:
https://mail.yourdomain.com/uploads/banner.png
```

That's it. Save the template and send. The URL stays as-is in the editor preview (your browser loads it normally over HTTP), but at send time the pipeline replaces it with a `cid:` reference and attaches the bytes inline.

### What you'll see in the raw email

If you inspect the source of a sent message, you'll see `multipart/related` instead of a flat `text/html` body:

```
Content-Type: multipart/related; type="text/html"; boundary="..."

--<boundary>
Content-Type: text/html; charset=utf-8
Content-Transfer-Encoding: quoted-printable

<html>...<img src="cid:abc123@billionmail">...</html>

--<boundary>
Content-Type: image/png
Content-Transfer-Encoding: base64
Content-ID: <abc123@billionmail>
Content-Disposition: inline

iVBORw0KGgoAAAANS...
```

---

## A note on updates

I'm not syncing this with upstream BillionMail. I got what I needed and moved on. If the official project ships something that breaks this, or ships their own CID support, I won't know and I won't fix it.

**Use it as-is.** If you need to adapt it, the relevant code is all in:

```
core/internal/service/mail_service/cid_embed.go   ← the rewrite logic
core/internal/service/mail_service/sending.go      ← MIME builder
core/internal/service/batch_mail/task_executor.go  ← batch campaign hook
core/internal/service/batch_mail/api_mail_send.go  ← transactional API hook
```

---

## Credits

[BillionMail](https://github.com/aaPanel/BillionMail) by aaPanel — the actual product. I just added one thing.

---

*MIT License — same as the original.*
