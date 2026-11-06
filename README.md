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
| React Native + TypeScript — planned | iOS-first mobile experience |
| LiveKit / WebRTC — planned | Voice communication after both players confirm |
| Docker Compose | Local PostgreSQL and Redis services |

The backend is a single service. Redis Lua scripts coordinate related matchmaking checks and updates atomically. WebSockets announce state changes; Redis remains the source of matchmaking state. The Go backend will authorize voice access rather than carry the audio stream itself.

## Current progress

The backend currently supports signup, Argon2id password hashing, JWT login, profile retrieval, game listing, queue entry/cancellation, pairing, confirmation, and automatic timeout/requeue behavior. WebSocket notifications cover pending matches, readiness, and timeout outcomes. These normal flows have been manually verified locally.

**V1 is not finished.** Mobile screens, voice integration, blocking/reporting, and public deployment remain ahead. Password reset is temporarily deferred while I focus on the core matching-to-voice journey. Cancellation notifications and reconnect snapshots for idle/searching states have been manually verified.

Development currently runs locally with the Go backend on the host and PostgreSQL/Redis in Docker Compose.
