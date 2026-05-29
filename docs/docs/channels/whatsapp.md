# WhatsApp Channel Configuration

Connecting your Memoh Bot to WhatsApp turns it into a personal/internal assistant
that talks to you the same way you talk to friends — directly inside the WhatsApp
mobile or desktop client. Memoh links to WhatsApp through whatsmeow, the same
multi-device protocol used by the official WhatsApp Web client. No phone number
re-registration is required, and no business API key is needed.

> WhatsApp's multi-device protocol is built for personal accounts and small
> internal groups. Do not use this channel for mass outreach or unsolicited
> messaging — WhatsApp will ban accounts that look spammy.

## Step 1: Add the WhatsApp channel

1. Open your bot's **Platforms** tab in the Memoh Web UI.
2. Click **Add Channel** and pick **WhatsApp**.

## Step 2: Link with QR code (default)

1. Click **Get QR Code**. Memoh creates a fresh whatsmeow session and prints a
   QR code.
2. On your phone, open WhatsApp -> **Settings -> Linked devices -> Link a
   device** and scan the code.
3. Confirm the link on your phone. The page automatically refreshes once the
   credentials are saved.

The QR code rotates every ~20 seconds. If it expires before you scan it, click
**Refresh QR Code** to start a new round.

## Step 2 (alternative): Link with phone code

If you cannot scan a QR code (for example, your network is unstable and the QR
keeps expiring), use the pair code flow instead:

1. Click **Get QR Code** to start a session.
2. On the same panel, click **Use pair code instead** and enter your full
   phone number with country code (digits only, e.g. `15551234567`).
3. Memoh returns an 8-character code formatted as `ABCD-EFGH`.
4. On your phone, open WhatsApp -> **Settings -> Linked devices -> Link with
   phone number** and type in the code.

## Step 3: Configure your account binding

In the channel settings, fill in **WhatsApp JID** for any user you want the bot
to recognize:

- A phone number, e.g. `15551234567` — Memoh normalizes it to the JID form.
- A direct JID, e.g. `15551234567@s.whatsapp.net`.
- A group JID, e.g. `120363000000000000@g.us`, when the bot should answer in a
  specific group.

## Optional settings

| Field | Description |
|-------|-------------|
| **Outbound Proxy** | Optional `http://`, `https://` or `socks5://` URL routed through both the WhatsApp websocket and media transports. Useful when WhatsApp servers are unreachable from your host. |
| **Client Display Name** | The "browser" name shown on the linked-devices list, e.g. `Chrome (Linux)`. WhatsApp validates this — only common browser/OS combinations are accepted. |

## Features supported

- **Text messages** in private chats and groups.
- **Group chats** — Memoh distinguishes individual senders within a group and
  preserves per-user memory.
- **Reply / quoted messages** — both incoming (the bot sees what you replied
  to) and outgoing (the bot quotes you when answering).
- **Inbound media** — images, videos, audio, voice notes, documents and
  stickers are auto-downloaded, decrypted by whatsmeow, and exposed to the bot
  as attachments (data URLs).
- **Outbound media** — bots can reply with images, videos, voice notes,
  documents, and so on; Memoh uploads to WhatsApp's CDN and forwards the
  encrypted reference.
- **Mention / reply detection** — `is_mentioned` and `is_reply_to_bot` are
  surfaced in routing metadata so ACL rules can keep the bot quiet in group
  chats unless explicitly addressed.
- **Logout handling** — when WhatsApp invalidates the session (you removed the
  linked device, or signed in elsewhere), the channel auto-disables itself.
  Re-link by running the QR / pair flow again.

## Troubleshooting

- **The QR code keeps expiring.** Try the pair code flow, or set an outbound
  proxy if your network is shaping WhatsApp traffic.
- **Inbound media is missing.** Memoh refuses to download attachments larger
  than 64 MiB to keep memory bounded. Smaller files always come through; for
  larger ones the message is delivered with size metadata only.
- **Group bot is too noisy.** Apply an ACL rule that requires
  `is_mentioned == true` or `is_reply_to_bot == true` for groups.
- **`whatsapp session is not logged in`.** Re-run the QR / pair flow. The
  underlying SQLite session store lives under
  `<data_root>/channels/whatsapp/<bot-id>.db` — deleting it forces a clean
  re-link.
