---
title: Push notifications
description: Send push notifications from your instance to the opendesk iPhone app, directly with your own APNs key or through the opt-in opendesk relay.
---

The [iPhone app](/guides/iphone-app/) receives notifications as push notifications through Apple's Push Notification service (APNs). Apple delivers a push only when it is sent with a key from the same Apple developer team that signed the app. A self-hosted instance therefore has three choices.

| Mode | Set | Reaches |
| --- | --- | --- |
| Off (default) | nothing | No pushes. The app works and updates live while it is open. |
| Relay | `PUSH_RELAY_URL=https://api.opendesk.sh` | The opendesk iPhone app from the App Store. |
| Direct | `APNS_KEY`, `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_TOPIC` | Only an iPhone app you build and sign yourself with that key's team. |

With an APNs key set, the instance sends directly and ignores `PUSH_RELAY_URL`. The [desktop app](/guides/desktop-app/) needs none of this: it shows notifications from the open connection.

To see which mode an instance is in:

```sh
curl -s https://desk.example.com/v1/auth/config | jq .native
```

```json
{ "app_attest": false, "push": "relay", "relay_url": "https://api.opendesk.sh" }
```

## Off

With neither a key nor a relay, the app's notification settings tell people that push is off on this server. Notifications still appear in the app and on the web, and the bell updates live.

## Relay

The relay is part of opendesk Cloud. It holds the APNs key for the App Store app and sends pushes on behalf of self-hosted instances that opt in. It is off until you set:

```sh
# .env
PUSH_RELAY_URL=https://api.opendesk.sh
```

then `docker compose up -d`.

### What the relay sees

Each push your instance asks the relay to send carries only:

- your instance's API address (`API_URL`),
- the notification type (`assigned`, `mentioned`, `customer_replied`, `sla_breached`) and the ticket number,
- the ids of the notification, ticket and workspace, which are random identifiers,
- the badge count,
- the phone's push token, sealed so that only the relay can read it.

It never receives ticket subjects or text, customer or agent names, or email addresses. The relay builds the notification itself from those fields, and the phone words it from the app's own text: what happened and the ticket number. When the notification is opened, the app loads the ticket from your instance directly.

### How relay requests are signed

1. When someone signs in on the phone, the app registers its push token with the relay and gets back a **relay token**: the push token and your instance's address, sealed with a key only the relay holds. The app gives your instance the relay token, never the push token itself.
2. Your instance signs every push it sends to the relay with its **instance key** (Ed25519), over the request body and a timestamp.
3. The relay opens the relay token and checks that it was issued for the instance that is sending. It then fetches your public key from `https://<your domain>/v1/push/instance-key` and checks the signature, refusing timestamps more than five minutes off.

So a relay token is useless to anyone except the instance it was issued for, and only the holder of your instance key can push to your users' phones.

The instance key is derived from `AUTH_SECRET`, so there is nothing extra to generate, store or back up. Changing `AUTH_SECRET` changes the key. The relay fetches public keys over HTTPS from public addresses only and keeps one for up to an hour, so allow up to an hour after a change for pushes to resume.

### Requirements

- `API_URL` is an `https` address that the internet can reach. The relay refuses private and loopback addresses, so an instance that is only on a private network cannot use it.
- `/v1/push/instance-key` is reachable at that address. The self-host Caddy configuration already sends it to opendesk.
- The instance can reach `https://api.opendesk.sh` on port 443.

## Direct

Direct mode sends from your instance to APNs with your own key. Apple accepts it only for an app signed by the key's team, so it reaches **an iPhone app you build and sign yourself**, not the App Store app. Use it if you distribute your own build of the app (`ios/` in the repository) to your team.

1. In the Apple Developer account that signs your build, create a key with **Apple Push Notifications service (APNs)** enabled (Certificates, Identifiers & Profiles, then Keys). Download the `.p8` file and note its key id.
2. Set in `.env`:

   ```sh
   APNS_KEY=LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0t...   # base64 < AuthKey_ABC123DEFG.p8 | tr -d '\n'
   APNS_KEY_ID=ABC123DEFG
   APNS_TEAM_ID=ABCDE12345
   APNS_TOPIC=com.example.opendesk     # your build's bundle id
   ```

   `APNS_KEY` is the `.p8` file, base64-encoded on one line, or its PEM text in double quotes with `\n` between lines. opendesk refuses to start with only some of `APNS_KEY`, `APNS_KEY_ID` and `APNS_TEAM_ID` set.
3. If Turnstile is on, also accept App Attest from your build: `APP_ATTEST_APP_IDS=ABCDE12345.com.example.opendesk`.
4. `docker compose up -d`.

Phones register with your instance, and pushes go from your instance to Apple. In direct mode the notification also shows the ticket subject. The instance needs outbound HTTPS to `api.push.apple.com` (and `api.sandbox.push.apple.com` for development builds).

## Who gets pushes

- A phone receives pushes only while its session is signed in. Signing out, a password change, a revoked session or removal from the workspace removes the phone at once.
- Each person can turn notification types off per phone in the app.
- When Apple reports that a phone no longer accepts pushes (the app was deleted), the instance forgets it.

See the [configuration reference](/self-hosting/configuration/#native-apps) for every setting.
