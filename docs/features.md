# WhatsRook Features

WhatsRook provides a complete, high-performance toolkit for interacting with WhatsApp and running autonomous bots.

## 1. Messaging & Media

- **Send & Receive Messages**: Full support for text messages, replies, extended text, and link previews.
- **Media Engine**: Download, decrypt, and upload media (images, audio, videos, documents, stickers).
- **Media Transcoding**: Integrated image transcoding (JPEG), Opus audio encoding for voice notes, and FFmpeg video conversions.
- **Interactive Messages**: Native polls, reactions, and interactive button payload handlers.
- **Quoted Context**: Extract text, media, and sender metadata from quoted/replied messages.

## 2. VoIP Calls & Media Playout (`wacaller`)

- **Anti-Call Automation**: Automatically reject incoming calls with customizable response messages or block lists.
- **Custom Call Media**: Stream custom audio (.opus) or video (.mp4) back to callers when receiving incoming voice or video calls.
- **NAT Traversal & SRTP**: Embedded STUN NAT traversal engine and SRTP/SRTCP encrypted media pipeline.

## 3. Group & Community Management

- **Group Actions**: Add, remove, promote, and demote participants; edit group subject, description, and permissions.
- **Group Caching**: Fast local SQLite / PostgreSQL cache for groups, communities, participants, and newsletters for low latency.
- **Event Listeners**: Automatic welcome and goodbye messages on participant join and leave events.
- **Activity Tracking**: Per-group user message counters and daily activity leaderboard.

## 4. Newsletters & Channels

- **Channel Sync**: Subscribe, follow, mute, and fetch updates from WhatsApp Newsletters.
- **Metadata Management**: Cache and query channel profile pictures, follower counts, and verification badges.

## 5. Bot Customization & Plugin Engine

- **Per-Session Settings**: All bot settings (prefix, bot name, sudoers, anticall, AFK, filters, games) are isolated per session (`our_jid`) in shared databases.
- **Extensible Plugins**: Modular plugin architecture (`cli/plugins`) covering:
  - **AI**: Gemini & Meta AI query integrations, vision analysis, prompt management.
  - **Tools**: Font styling (`fancy` with support for replied messages), URL screenshots, sticker generators.
  - **Games**: Tic-Tac-Toe, Word Scramble, Word Chain Game (WCG), XP points tracking.
  - **Filters & BGM**: Trigger-based automated message responses and audio background music playout.
  - **Administration**: Sudoers management, bot restarts, runtime updater.

## 6. Realtime WebSocket & JSON API

- **JSON Framing**: Clean, lightweight WebSocket hub streaming JSON events (`message`, `incoming_call`, `connected`, `stats`, `pair_code`, etc.).
- **Remote Control**: Send messages, reactions, edits, and revokes programmatically through WebSocket control frames.
