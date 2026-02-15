# Music Intent Coordinator

A centralized music intent coordinator for home automation that eliminates duplication by managing playlists and speaker mappings in a single SQLite database.

## Purpose

- **Eliminate Duplication**: Playlists and speaker IDs are no longer hardcoded across multiple automations
- **Centralize Configuration**: One database holds all mappings (intent → playlist, location → speaker entity)
- **Decouple Intent from Execution**: Home Assistant (with Music Assistant) becomes the executor, not the brain
- **Make Changes Low-Friction**: Change a playlist or speaker once in the DB → all automations automatically respect it

## Architecture

```
┌─────────────────┐
│  Home Assistant │
│   Automations   │
└────────┬────────┘
         │ HTTP POST
         │ {intent: "christmas", location: "garage"}
         ▼
┌─────────────────┐
│   Coordinator   │
│   (Go Service)  │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌────────┐ ┌──────────────┐
│ SQLite │ │ Home        │
│   DB   │ │ Assistant   │
│        │ │ API         │
└────────┘ └──────────────┘
```

## Components

### SQLite Database

Two tables:
- **intent**: Maps intent names to playlists (supports multiple playlists per intent)
  - `name` (TEXT, UNIQUE): Intent identifier (e.g., "christmas", "saturday_morning")
  - `playlist` (TEXT): Playlist URI(s) stored as JSON array - supports multiple playlists for random selection
- **location**: Maps location names to speaker entities
  - `name` (TEXT, UNIQUE): Location identifier (e.g., "garage", "living_room")
  - `speaker_entity` (TEXT): Home Assistant media player entity ID

### Coordinator Service (Go)

- **MQTT-based communication**: Listens for play requests on MQTT topic `music-coordinator/play`
- **HTTP API**: Web UI and REST API for CRUD operations (still available)
- Looks up playlist and speaker entity in SQLite
- Publishes play commands to Home Assistant via MQTT
- Stateless; DB is the authoritative state

## Setup

### Prerequisites

- Go 1.21 or later
- Home Assistant with Music Assistant (mass) integration
- Home Assistant API token

### Installation

1. Clone or navigate to the music-coordinator directory:
```bash
cd music-coordinator
```

2. Install dependencies:
```bash
go mod download
```

3. Set environment variables:
```bash
export HA_API_TOKEN="your-home-assistant-long-lived-access-token"
export HA_URL="http://homeassistant.local:8123"  # or your HA URL
export PORT="8080"  # optional, defaults to 8080
export DB_PATH="./music_coordinator.db"  # optional, defaults to ./music_coordinator.db
```

4. Build and run:
```bash
go build -o music-coordinator
./music-coordinator
```

Or run directly:
```bash
go run main.go
```

### Initialize Database

The database schema is automatically created on first run. To populate with example data:

```bash
sqlite3 music_coordinator.db < init_db.sql
```

Or manually add entries:
```sql
INSERT INTO intent (name, playlist) VALUES ('christmas', 'spotify:user:spotify:playlist:37i9dQZF1DXdd3gw5QVjt9');
INSERT INTO location (name, speaker_entity) VALUES ('garage', 'media_player.garage');
```

## Web UI

The coordinator includes a simple web-based CRUD interface for managing intents and locations.

### Access the UI

Once the service is running, open your browser and navigate to:
```
http://localhost:8080/
```

### Features

- **View all intents and locations** in a clean table format
- **Create new intents** with multiple playlists - Enter multiple playlists (one per line) and the system will randomly select one each time
- **Create new locations** by entering a name and speaker entity ID
- **Edit existing entries** by clicking the "Edit" button
- **Delete entries** by clicking the "Delete" button (with confirmation)
- **Sync locations from Home Assistant** - Automatically fetch all media players from Home Assistant and create locations for them
- **Browse available media players** - See all media players from Home Assistant in a dropdown and list
- **Random playlist selection** - When an intent has multiple playlists, one is randomly selected each time the intent is used

The UI automatically refreshes after create/update/delete operations.

## Usage

### MQTT Communication

The coordinator uses MQTT for all music playback commands. This provides better integration with Home Assistant and eliminates the need for HTTP API tokens.

#### Subscribe to Play Requests

The coordinator automatically subscribes to:
- **Topic**: `music-coordinator/play`
- **QoS**: 0
- **Message Format**: JSON
  ```json
  {
    "intent": "christmas",
    "location": "garage"
  }
  ```

#### Publish Play Commands to Home Assistant

The coordinator publishes play commands to:
- **Topic**: `homeassistant/service/mass/play_media`
- **QoS**: 0
- **Message Format**: JSON
  ```json
  {
    "entity_id": "media_player.garage",
    "media_id": "spotify:playlist:37i9dQZF1DXdd3gw5QVjt9",
    "media_type": "playlist"
  }
  ```

### HTTP API Endpoints (Web UI)

The HTTP API is still available for the web UI and CRUD operations:

#### Play Music (HTTP)

**POST** `/api/play`

Request body:
```json
{
  "intent": "christmas",
  "location": "garage"
}
```

Response (success):
```json
{
  "success": true,
  "message": "Playing intent 'christmas' on 'garage' (playlist: spotify:user:spotify:playlist:37i9dQZF1DXdd3gw5QVjt9, speaker: media_player.garage)"
}
```

Response (error):
```json
{
  "success": false,
  "error": "Intent not found: intent 'unknown' not found"
}
```

### CRUD API Endpoints

#### Intents

- **GET** `/api/intents` - List all intents
- **GET** `/api/intents/{name}` - Get a specific intent
- **POST** `/api/intents` - Create a new intent
  ```json
  {
    "name": "christmas",
    "playlist": "spotify:playlist:37i9dQZF1DXdd3gw5QVjt9"
  }
  ```
- **PUT** `/api/intents/{name}` - Update an intent
  ```json
  {
    "playlist": "spotify:playlist:NEW_ID"
  }
  ```
- **DELETE** `/api/intents/{name}` - Delete an intent

#### Locations

- **GET** `/api/locations` - List all locations
- **GET** `/api/locations/{name}` - Get a specific location
- **POST** `/api/locations` - Create a new location
  ```json
  {
    "name": "garage",
    "speaker_entity": "media_player.garage"
  }
  ```
- **PUT** `/api/locations/{name}` - Update a location
  ```json
  {
    "speaker_entity": "media_player.new_speaker"
  }
  ```
- **DELETE** `/api/locations/{name}` - Delete a location

#### Media Players & Sync

- **GET** `/api/media-players` - Get all media player entities from Home Assistant
  Returns a list of media players with entity_id, name, state, and device_name
- **POST** `/api/sync-locations` - Sync locations from Home Assistant
  Automatically creates locations for all media players that don't already exist.
  Location names are derived from entity IDs (e.g., `media_player.garage` → `garage`)

### Health Check

**GET** `/health`

Returns `200 OK` if the service is running.

## Home Assistant Integration

### Using MQTT (Recommended)

The coordinator listens for play requests on MQTT topic `music-coordinator/play`:

```yaml
automation:
  - alias: "Play Christmas Music in Garage"
    trigger:
      - platform: state
        entity_id: input_boolean.christmas_mode
        to: 'on'
    action:
      - service: mqtt.publish
        data:
          topic: "music-coordinator/play"
          payload: |
            {
              "intent": "christmas",
              "location": "garage"
            }
```

See `examples/mqtt_automation.yaml` for more examples.

### Using HTTP API (Alternative)

You can still use HTTP API if preferred:

```yaml
automation:
  - alias: "Play Christmas Music in Garage"
    trigger:
      - platform: state
        entity_id: input_boolean.christmas_mode
        to: 'on'
    action:
      - service: http.post
        data:
          url: "http://music-coordinator:8080/api/play"
          method: POST
          headers:
            Content-Type: "application/json"
          data:
            intent: "christmas"
            location: "garage"
```

### Using REST Command (Recommended)

Add to `configuration.yaml`:

```yaml
rest_command:
  play_music_intent:
    url: "http://music-coordinator:8080/api/play"
    method: POST
    headers:
      Content-Type: "application/json"
    payload: |
      {
        "intent": "{{ intent }}",
        "location": "{{ location }}"
      }
```

Then use in automations:

```yaml
automation:
  - alias: "Play Christmas Music in Garage"
    trigger:
      - platform: state
        entity_id: input_boolean.christmas_mode
        to: 'on'
    action:
      - service: rest_command.play_music_intent
        data:
          intent: "christmas"
          location: "garage"
```

## Database Management

### Using the Web UI (Recommended)

The easiest way to manage the database is through the web UI at `http://localhost:8080/`:

1. **Sync Locations from Home Assistant**: Click the "Sync from Home Assistant" button to automatically create locations for all your media players
2. **Browse Available Media Players**: The UI shows all media players from Home Assistant in a dropdown and list
3. **Select from Dropdown**: When creating a location, select a media player from the dropdown instead of typing

### Using SQL (Advanced)

If you prefer to manage the database directly:

#### View all intents:
```sql
SELECT * FROM intent;
```

#### View all locations:
```sql
SELECT * FROM location;
```

#### Add new intent:
```sql
INSERT INTO intent (name, playlist) VALUES ('workout', 'spotify:user:spotify:playlist:YOUR_PLAYLIST_ID');
```

#### Add new location:
```sql
INSERT INTO location (name, speaker_entity) VALUES ('office', 'media_player.office_speaker');
```

#### Update existing intent:
```sql
UPDATE intent SET playlist = 'spotify:user:spotify:playlist:NEW_ID' WHERE name = 'christmas';
```

#### Update existing location:
```sql
UPDATE location SET speaker_entity = 'media_player.new_speaker' WHERE name = 'garage';
```

### Using the API

#### Sync locations from Home Assistant:
```bash
curl -X POST http://localhost:8080/api/sync-locations
```

#### Get all media players from Home Assistant:
```bash
curl http://localhost:8080/api/media-players
```

## Docker Deployment

Create a `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o music-coordinator

FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
WORKDIR /app
COPY --from=builder /app/music-coordinator .
COPY --from=builder /app/init_db.sql .
EXPOSE 8080
CMD ["./music-coordinator"]
```

Build and run:
```bash
docker build -t music-coordinator .
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/music_coordinator.db:/app/music_coordinator.db \
  -e HA_API_TOKEN="your-token" \
  -e HA_URL="http://homeassistant.local:8123" \
  music-coordinator
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8082` | HTTP server port (for web UI) |
| `DB_PATH` | `./music_coordinator.db` | SQLite database file path |
| `MQTT_BROKER` | `tcp://localhost:1883` | MQTT broker URL (required) |
| `MQTT_USER` | `` | MQTT username (optional) |
| `MQTT_PASS` | `` | MQTT password (optional) |
| `MQTT_CLIENT_ID` | `music-coordinator` | MQTT client ID |
| `HA_URL` | `https://ha.bryanwp.com` | Home Assistant base URL (only needed for media player sync) |
| `HA_API_TOKEN` | `` | Home Assistant long-lived access token (only needed for media player sync) |
| `MA_API_URL` | `http://192.168.2.245:8097` | Music Assistant API URL (currently unused) |

## Troubleshooting

### "Intent not found" error
- Check that the intent exists in the database: `SELECT * FROM intent WHERE name = 'your_intent';`
- Ensure the intent name matches exactly (case-sensitive)

### "Location not found" error
- Check that the location exists: `SELECT * FROM location WHERE name = 'your_location';`
- Ensure the location name matches exactly (case-sensitive)

### "Failed to play music" error
- Verify Home Assistant API token is correct
- Check that the speaker entity ID exists in Home Assistant
- Ensure Music Assistant (mass) integration is properly configured
- Check Home Assistant logs for more details

### Database locked errors
- Ensure only one instance of the coordinator is running
- Check file permissions on the database file

## License

MIT

