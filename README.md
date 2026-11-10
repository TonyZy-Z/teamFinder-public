# TeamFinder

TeamFinder is an ongoing personal project: an iOS-first app that helps people find a teammate for the same video game and talk through voice chat while playing.

I’m building it to become a full-stack engineer, with a particular focus on excelling at **Go**. The goal is to understand and implement a complete product—from HTTP APIs and authentication to concurrent matchmaking, real-time communication, a mobile client, and deployment.

## Version 1 goal

Deliver one complete user journey:

```text
Create an account → choose a game → find a teammate
    → both confirm within 30 seconds → join voice chat
    → mute/unmute or leave → search again
```

V1 targets one-to-one, game-specific matchmaking in arrival order, with support for **Civilization VI**, **Battlefield 6**, and **Ready or Not**. Users should be able to cancel a search, reset a password, and report or block another player. Blocked players must not be matched together.

The priority is a small, working end-to-end product. Features such as friends, rankings, text chat, and group matchmaking are outside V1.

## Architecture at a glance

| Component | Role |
|---|---|
| Go backend | HTTP APIs, authentication, matchmaking rules, and WebSocket notifications |
| PostgreSQL | Persistent account data and future safety records |
| Redis | Temporary queues, match assignments, and confirmation deadlines |
| React Native + TypeScript, with Expo | iOS-first mobile client; login and profile retrieval are working |
| LiveKit Cloud / WebRTC | Authorized voice rooms; audio verified through a browser test page |
| Docker Compose | Local PostgreSQL and Redis services |

The backend is a single service. Redis Lua scripts coordinate related matchmaking checks and updates atomically. WebSockets announce state changes; Redis remains the source of matchmaking state. The Go backend authorizes voice access and manages room cleanup; clients exchange audio through LiveKit rather than through the Go service.

## Current progress

Last updated: **November 10, 2026**.

Implemented and manually verified locally:

- **Backend authentication:** signup, Argon2id password hashing, JWT login, and protected profile retrieval, with accounts stored in PostgreSQL.
- **Matchmaking:** game listing, queue entry/cancellation, same-game pairing, a 30-second confirmation window, ready matches, and timeout/requeue behavior.
- **Real-time updates:** WebSocket notifications for pairing, readiness, cancellation, and timeout outcomes, plus idle/searching snapshots on reconnect.
- **Voice:** room-scoped tokens for ready-match participants, both participants joining the same LiveKit room, and two-way microphone audio using two browser tabs on one Mac.
- **Intentional leave:** ending the match releases both users' Redis assignments, closes the voice room, and revokes participant tokens. Repeated leave requests and rejection of saved tokens after revocation have been tested.
- **Mobile authentication:** the Expo app runs in the iOS Simulator. The login screen validates required fields, displays loading/errors, calls the Go backend, saves its access token in SecureStore, and retrieves the signed-in user's profile. Wrong-password and successful-login paths have been tested.

The mobile code separates route entries, feature screens, API calls, token storage, and session operations. The Redux authentication slice, store, and typed hooks are implemented and text-reviewed, but are not yet connected to the UI. The next step is adding the Redux Provider, followed by session restoration, authenticated navigation, and logout.

**V1 is not finished.** Mobile signup, game selection, matchmaking screens, real-time updates, and native LiveKit integration remain to be connected. Blocking/reporting and public deployment are also unfinished. Password reset is deferred while I focus on the core matching-to-voice journey. Microphone mute/unmute has not been verified yet.

Automatic cleanup after an unexpected disconnect and two-device audio testing are explicitly deferred until after frontend implementation and deployment. The current voice evidence comes from the browser test page, not the React Native client. Automated testing and broader reliability work remain future improvements.

Development currently runs locally with the Go backend on the host and PostgreSQL/Redis in Docker Compose.
