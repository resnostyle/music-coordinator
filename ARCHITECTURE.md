# Architecture Overview

## System Design

The Music Intent Coordinator is a stateless HTTP service that acts as a translation layer between high-level music intents and low-level Home Assistant API calls.

```
┌─────────────────────────────────────────────────────────────┐
│                    Home Assistant Automations                 │
│  (Emit high-level intents: "christmas", "garage")           │
└───────────────────────┬─────────────────────────────────────┘
                        │ HTTP POST
                        │ {intent: "christmas", location: "garage"}
                        ▼
┌─────────────────────────────────────────────────────────────┐
│              Music Intent Coordinator (Go)                    │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  HTTP Server (Port 8080)                               │  │
│  │  - /play endpoint                                      │  │
│  │  - /health endpoint                                    │  │
│  └────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Database Layer (SQLite)                               │  │
│  │  - intent table: name → playlist                      │  │
│  │  - location table: name → speaker_entity              │  │
│  └────────────────────────────────────────────────────────┘  │
│  ┌────────────────────────────────────────────────────────┐  │
│  │  Home Assistant Client                                 │  │
│  │  - Calls mass.play_media service                       │  │
│  └────────────────────────────────────────────────────────┘  │
└───────────────────────┬─────────────────────────────────────┘
                        │ REST API Call
                        │ POST /api/services/mass/play_media
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

1. **Automation Trigger**: Home Assistant automation triggers (e.g., Christmas mode enabled)
2. **Intent Request**: Automation calls coordinator with `{intent: "christmas", location: "garage"}`
3. **Database Lookup**: Coordinator queries SQLite:
   - `intent` table: `"christmas"` → `"spotify:playlist:37i9dQZF1DXdd3gw5QVjt9"`
   - `location` table: `"garage"` → `"media_player.garage"`
4. **API Call**: Coordinator calls Home Assistant API:
   ```json
   POST /api/services/mass/play_media
   {
     "entity_id": "media_player.garage",
     "media_id": "spotify:playlist:37i9dQZF1DXdd3gw5QVjt9",
     "media_type": "playlist"
   }
   ```
5. **Music Plays**: Music Assistant plays the playlist on the specified speaker

## Database Schema

### `intent` Table
| Column | Type | Description |
|--------|------|-------------|
| id | INTEGER PRIMARY KEY | Auto-increment ID |
| name | TEXT UNIQUE | Intent identifier (e.g., "christmas", "saturday_morning") |
| playlist | TEXT | Playlist URI or identifier |
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

## API Endpoints

### POST /play
Plays music based on intent and location.

**Request:**
```json
{
  "intent": "christmas",
  "location": "garage"
}
```

**Success Response (200):**
```json
{
  "success": true,
  "message": "Playing intent 'christmas' on 'garage' (playlist: spotify:playlist:..., speaker: media_player.garage)"
}
```

**Error Response (400/404/500):**
```json
{
  "success": false,
  "error": "Intent not found: intent 'unknown' not found"
}
```

### GET /health
Health check endpoint.

**Response (200):**
```
OK
```

## Benefits

1. **Single Source of Truth**: All playlist and speaker mappings in one database
2. **Easy Updates**: Change playlist once, all automations use new playlist
3. **Decoupling**: Automations don't need to know specific playlists or speakers
4. **Maintainability**: No hardcoded values scattered across automations
5. **Flexibility**: Easy to add new intents/locations without changing automations

## Extension Points

The architecture supports future enhancements:

1. **Multiple Playlists per Intent**: Could add support for random selection or time-based selection
2. **Intent Priority**: Could add priority levels for conflicting requests
3. **Volume Control**: Could add volume mapping per location
4. **Scheduling**: Could add time-based playlist selection
5. **Direct MA API**: Could bypass Home Assistant and call Music Assistant API directly

