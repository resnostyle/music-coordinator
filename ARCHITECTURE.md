# Architecture Overview

## System Design

The Music Intent Coordinator is a lightweight Go service that acts as a translation layer between high-level music intents and Home Assistant/Music Assistant playback commands via MQTT.

```
┌─────────────────────────────────────────────────────────────┐
│                    Home Assistant Automations                 │
│  (Emit high-level intents: "christmas", "garage")           │
└───────────────────────┬─────────────────────────────────────┘
                        │ MQTT publish
                        │ {intent: "christmas", location: "garage"}
                        ▼
┌─────────────────────────────────────────────────────────────┐
│              Music Intent Coordinator (Go)                    │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  HTTP Server (Port 8080)                               │  │
│  │  - /api/play endpoint                                  │  │
│  │  - CRUD endpoints for intents/locations/groups         │  │
│  │  - Web UI                                              │  │
│  │  - /health endpoint                                    │  │
│  └────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  MQTT Client                                           │  │
│  │  - Subscribes to: music-coordinator/play               │  │
│  │  - Publishes to: homeassistant/service/mass/play_media │  │
│  └────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Database Layer (SQLite)                               │  │
│  │  - intent table: name → playlist(s)                   │  │
│  │  - location table: name → speaker_entity              │  │
│  │  - playlist_group: reusable sets of playlists         │  │
│  └────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Home Assistant Client (optional)                      │  │
│  │  - Fetches media player entities for sync              │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
                        │ MQTT publish
                        │ homeassistant/service/mass/play_media
                        ▼
┌─────────────────────────────────────────────────────────────┐
│                    Home Assistant                             │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Music Assistant (mass) Integration                    │  │
│  │  - Receives: entity_id, media_id, media_type           │  │
│  │  - Plays playlist on specified speaker                 │  │
│  └────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Data Flow

1. **Automation Trigger**: Home Assistant automation fires (e.g., Christmas mode enabled)
2. **Intent Request**: Automation publishes to MQTT topic `music-coordinator/play`:
   ```json
   {"intent": "christmas", "location": "garage"}
   ```
3. **Database Lookup**: Coordinator queries SQLite:
   - `intent` table: `"christmas"` → randomly selects from configured playlists
   - `location` table: `"garage"` → `"media_player.garage"`
4. **MQTT Publish**: Coordinator publishes to `homeassistant/service/mass/play_media`:
   ```json
   {
     "entity_id": "media_player.garage",
     "media_id": "spotify:playlist:SELECTED_PLAYLIST_ID",
     "media_type": "playlist"
   }
   ```
5. **Music Plays**: Music Assistant receives the command and plays the playlist on the speaker

## Database Schema

### `intent` Table
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER PRIMARY KEY | Auto-increment ID |
| name | TEXT UNIQUE | Intent identifier (e.g., "christmas", "workout") |
| playlist | TEXT | Playlist URI(s) as JSON array |
| playlist_group | TEXT | Optional reference to a playlist_group name |
| created_at | DATETIME | Creation timestamp |
| updated_at | DATETIME | Last update timestamp |

### `location` Table
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER PRIMARY KEY | Auto-increment ID |
| name | TEXT UNIQUE | Location identifier (e.g., "garage", "living_room") |
| speaker_entity | TEXT | Home Assistant media player entity ID |
| created_at | DATETIME | Creation timestamp |
| updated_at | DATETIME | Last update timestamp |

### `playlist_group` Table
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER PRIMARY KEY | Auto-increment ID |
| name | TEXT UNIQUE | Group identifier |
| created_at | DATETIME | Creation timestamp |
| updated_at | DATETIME | Last update timestamp |

### `playlist_group_item` Table
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER PRIMARY KEY | Auto-increment ID |
| group_name | TEXT | Foreign key → playlist_group.name (CASCADE delete) |
| playlist | TEXT | Playlist URI |
| created_at | DATETIME | Creation timestamp |

## Benefits

1. **Single Source of Truth**: All playlist and speaker mappings in one database
2. **Easy Updates**: Change a playlist once, all automations use the new playlist
3. **Decoupling**: Automations don't need to know specific playlists or speakers
4. **Maintainability**: No hardcoded values scattered across automations
5. **Flexibility**: Easy to add new intents/locations without changing automations

## Extension Points

The architecture supports future enhancements:

1. **Intent Priority**: Priority levels for conflicting requests
2. **Volume Control**: Volume mapping per location
3. **Scheduling**: Time-based playlist selection
4. **Direct MA API**: Bypass Home Assistant and call Music Assistant API directly
